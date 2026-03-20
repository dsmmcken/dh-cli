//go:build !linux

package cmd

func checkUffd() CheckResult {
	return CheckResult{
		Name:   "VM/UFFD",
		Status: "ok",
		Detail: "not applicable (VM requires Linux)",
	}
}

func fixUffd() error {
	return nil
}
