package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dsmmcken/dh-cli/go_src/internal/config"
	"github.com/dsmmcken/dh-cli/go_src/internal/java"
	"github.com/dsmmcken/dh-cli/go_src/internal/output"
	"github.com/dsmmcken/dh-cli/go_src/internal/versions"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

var fixFlag bool

func addDoctorCommand(parent *cobra.Command) {
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check environment health",
		Long:  "Run diagnostic checks across all subsystems and report environment health.",
		Args:  cobra.NoArgs,
		RunE:  runDoctor,
	}

	doctorCmd.Flags().BoolVar(&fixFlag, "fix", false, "Attempt to auto-fix problems")

	parent.AddCommand(doctorCmd)
}

// CheckResult holds the result of a single doctor check.
type CheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok", "warning", "error"
	Detail string `json:"detail"`
}

// DoctorReport holds the complete doctor output.
type DoctorReport struct {
	Healthy bool          `json:"healthy"`
	Checks  []CheckResult `json:"checks"`
}

// Testable check functions — replaceable in unit tests.
var (
	UVChecker             = checkUV
	JavaChecker           = checkJava
	VersionsChecker       = checkVersions
	DefaultVersionChecker = checkDefaultVersion
	DiskSpaceChecker      = checkDiskSpace
)

func runDoctor(cmd *cobra.Command, args []string) error {
	config.SetConfigDir(ConfigDir)
	dhgHome := config.DHGHome()

	checks := []CheckResult{
		UVChecker(),
		JavaChecker(dhgHome),
		VersionsChecker(dhgHome),
		DefaultVersionChecker(dhgHome),
		DiskSpaceChecker(dhgHome),
	}

	healthy := true
	for _, c := range checks {
		if c.Status == "error" {
			healthy = false
			break
		}
	}

	report := DoctorReport{
		Healthy: healthy,
		Checks:  checks,
	}

	if output.IsJSON() {
		return output.PrintJSON(cmd.OutOrStdout(), report)
	}

	// Human output
	if output.IsQuiet() && healthy {
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Deephaven CLI Doctor")
	fmt.Fprintln(cmd.OutOrStdout())

	var warnings, errors int
	for _, c := range checks {
		symbol := "\u2713" // checkmark
		switch c.Status {
		case "warning":
			symbol = "\u26A0" // warning triangle
			warnings++
		case "error":
			symbol = "\u2717" // X mark
			errors++
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s %-12s %s\n", symbol, c.Name, c.Detail)
	}

	fmt.Fprintln(cmd.OutOrStdout())

	if errors > 0 {
		var parts []string
		parts = append(parts, pluralize(errors, "error"))
		if warnings > 0 {
			parts = append(parts, pluralize(warnings, "warning"))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Problems found (%s).\n", strings.Join(parts, ", "))
	} else if warnings > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Everything looks good (%s).\n", pluralize(warnings, "warning"))
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Everything looks good.")
	}

	if fixFlag {
		runFixes(cmd, checks, dhgHome)
	}

	return nil
}

func pluralize(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func checkUV() CheckResult {
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return CheckResult{
			Name:   "uv",
			Status: "error",
			Detail: "not found in PATH",
		}
	}

	out, err := exec.Command(uvPath, "--version").Output()
	if err != nil {
		return CheckResult{
			Name:   "uv",
			Status: "error",
			Detail: fmt.Sprintf("found at %s but failed to get version", uvPath),
		}
	}

	version := strings.TrimSpace(string(out))
	// uv --version outputs "uv 0.5.14" — extract just the version
	version = strings.TrimPrefix(version, "uv ")

	return CheckResult{
		Name:   "uv",
		Status: "ok",
		Detail: fmt.Sprintf("%s (%s)", uvPath, version),
	}
}

func checkJava(dhgHome string) CheckResult {
	info, err := java.Detect(dhgHome)
	if err != nil {
		return CheckResult{
			Name:   "Java",
			Status: "error",
			Detail: fmt.Sprintf("detection failed: %s", err),
		}
	}

	if !info.Found {
		return CheckResult{
			Name:   "Java",
			Status: "error",
			Detail: "not found",
		}
	}

	if !java.MeetsMinimum(info.Version, java.MinimumVersion) {
		return CheckResult{
			Name:   "Java",
			Status: "warning",
			Detail: fmt.Sprintf("%s (%s) — below minimum %d", info.Version, info.Source, java.MinimumVersion),
		}
	}

	return CheckResult{
		Name:   "Java",
		Status: "ok",
		Detail: fmt.Sprintf("%s (%s)", info.Version, info.Source),
	}
}

func checkVersions(dhgHome string) CheckResult {
	installed, err := versions.ListInstalled(dhgHome)
	if err != nil {
		return CheckResult{
			Name:   "Versions",
			Status: "warning",
			Detail: fmt.Sprintf("could not list: %s", err),
		}
	}

	count := len(installed)
	if count == 0 {
		return CheckResult{
			Name:   "Versions",
			Status: "warning",
			Detail: "0 installed",
		}
	}

	return CheckResult{
		Name:   "Versions",
		Status: "ok",
		Detail: fmt.Sprintf("%d installed", count),
	}
}

func checkDefaultVersion(dhgHome string) CheckResult {
	cfg, err := config.Load()
	if err != nil {
		return CheckResult{
			Name:   "Default",
			Status: "error",
			Detail: fmt.Sprintf("could not load config: %s", err),
		}
	}

	if cfg.DefaultVersion == "" {
		return CheckResult{
			Name:   "Default",
			Status: "error",
			Detail: "not set",
		}
	}

	// Check if the default version exists on disk
	vDir := filepath.Join(dhgHome, "versions", cfg.DefaultVersion)
	info, err := os.Stat(vDir)
	if err != nil || !info.IsDir() {
		return CheckResult{
			Name:   "Default",
			Status: "error",
			Detail: fmt.Sprintf("%s (not installed)", cfg.DefaultVersion),
		}
	}

	return CheckResult{
		Name:   "Default",
		Status: "ok",
		Detail: cfg.DefaultVersion,
	}
}

func checkDiskSpace(dhgHome string) CheckResult {
	var stat unix.Statfs_t
	target := dhgHome
	if _, err := os.Stat(target); err != nil {
		target = filepath.Dir(target)
	}
	if err := unix.Statfs(target, &stat); err != nil {
		return CheckResult{
			Name:   "Disk",
			Status: "warning",
			Detail: fmt.Sprintf("could not check: %s", err),
		}
	}

	freeBytes := stat.Bavail * uint64(stat.Bsize)
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)

	status := "ok"
	if freeGB < 5.0 {
		status = "warning"
	}

	displayPath := shortenHome(dhgHome)

	return CheckResult{
		Name:   "Disk",
		Status: status,
		Detail: fmt.Sprintf("%.1f GB free in %s", freeGB, displayPath),
	}
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func runFixes(cmd *cobra.Command, checks []CheckResult, dhgHome string) {
	for _, c := range checks {
		if c.Status == "ok" {
			continue
		}
		switch c.Name {
		case "uv":
			if c.Status == "error" {
				fmt.Fprintln(cmd.OutOrStdout(), "\nFix: Install uv with: curl -LsSf https://astral.sh/uv/install.sh | sh")
			}
		case "Java":
			if c.Status == "error" {
				fmt.Fprintln(cmd.OutOrStdout(), "\nFix: Run 'dhg java install' to install Java.")
			}
		case "Versions":
			if c.Status == "warning" && strings.Contains(c.Detail, "0 installed") {
				fmt.Fprintln(cmd.OutOrStdout(), "\nFix: Run 'dhg install' to install a Deephaven version.")
			}
		case "Default":
			if c.Status == "error" {
				installed, err := versions.ListInstalled(dhgHome)
				if err == nil && len(installed) > 0 {
					latest := installed[0].Version
					if err := config.Set("default_version", latest); err == nil {
						fmt.Fprintf(cmd.OutOrStdout(), "\nFix: Set default version to %s.\n", latest)
					}
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "\nFix: Install a version first with 'dhg install', then run 'dhg use <version>'.")
				}
			}
		}
	}
}
