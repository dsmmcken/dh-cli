package tests

import (
	"testing"

	"github.com/dsmmcken/dh-cli/src/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProcNetTCPContent(t *testing.T) {
	// Fixture data from a real /proc/net/tcp
	fixture := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:2710 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12346 1 0000000000000000 100 0 0 10 0
   2: 0100007F:0050 0100007F:C000 01 00000000:00000000 00:00000000 00000000  1000        0 12347 1 0000000000000000 100 0 0 10 0
`
	entries := discovery.ParseProcNetTCPContent(fixture)
	require.Len(t, entries, 3)

	// Entry 0: port 10000 (0x2710), state LISTEN (0x0A), inode 12345
	assert.Equal(t, 10000, entries[0].Port)
	assert.Equal(t, uint64(12345), entries[0].Inode)
	assert.Equal(t, 0x0A, entries[0].State)

	// Entry 1: port 8080 (0x1F90), state LISTEN (0x0A), inode 12346
	assert.Equal(t, 8080, entries[1].Port)
	assert.Equal(t, uint64(12346), entries[1].Inode)
	assert.Equal(t, 0x0A, entries[1].State)

	// Entry 2: port 80 (0x0050), state ESTABLISHED (0x01), inode 12347
	assert.Equal(t, 80, entries[2].Port)
	assert.Equal(t, uint64(12347), entries[2].Inode)
	assert.Equal(t, 0x01, entries[2].State)
}

func TestParseProcNetTCPContentEmpty(t *testing.T) {
	fixture := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
`
	entries := discovery.ParseProcNetTCPContent(fixture)
	assert.Empty(t, entries)
}

func TestParseProcNetTCPContentIPv6(t *testing.T) {
	fixture := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:2710 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 54321 1 0000000000000000 100 0 0 10 0
`
	entries := discovery.ParseProcNetTCPContent(fixture)
	require.Len(t, entries, 1)
	assert.Equal(t, 10000, entries[0].Port)
	assert.Equal(t, uint64(54321), entries[0].Inode)
	assert.Equal(t, 0x0A, entries[0].State)
}

func TestParseDockerPSOutput(t *testing.T) {
	fixture := "abc123def456\tghcr.io/deephaven/server:latest\t0.0.0.0:10000->10000/tcp\n" +
		"def789abc012\tnginx:latest\t0.0.0.0:80->80/tcp\n" +
		"111222333444\tdeephaven/server:0.35.1\t0.0.0.0:10000->10000/tcp, 0.0.0.0:10001->10001/tcp\n"

	servers := discovery.ParseDockerPSOutput(fixture)

	// nginx should be filtered out
	require.Len(t, servers, 3) // 1 from first + 2 from third

	assert.Equal(t, 10000, servers[0].Port)
	assert.Equal(t, "docker", servers[0].Source)
	assert.Equal(t, "abc123def456", servers[0].ContainerID)
	assert.Equal(t, "ghcr.io/deephaven/server:latest", servers[0].Script)

	assert.Equal(t, 10000, servers[1].Port)
	assert.Equal(t, "111222333444", servers[1].ContainerID)

	assert.Equal(t, 10001, servers[2].Port)
	assert.Equal(t, "111222333444", servers[2].ContainerID)
}

func TestParseDockerPSOutputEmpty(t *testing.T) {
	servers := discovery.ParseDockerPSOutput("")
	assert.Empty(t, servers)
}

func TestParseDockerPSOutputNoDeephaven(t *testing.T) {
	fixture := "abc123\tnginx:latest\t0.0.0.0:80->80/tcp\n"
	servers := discovery.ParseDockerPSOutput(fixture)
	assert.Empty(t, servers)
}

func TestParseLsofOutput(t *testing.T) {
	// Only processes whose comm matches a Deephaven pattern should appear.
	// "java" alone is not enough — needs "io.deephaven" in cmdline.
	// "python3" alone is not Deephaven, so it should be filtered out.
	// "dh-serve" matches the dh serve pattern.
	fixture := "p1234\ncjava\nn*:10000\np5678\ncpython3\nn127.0.0.1:8080\np9999\ncdh-serve\nn*:10001\n"
	servers := discovery.ParseLsofOutput(fixture)
	require.Len(t, servers, 1)

	assert.Equal(t, 10001, servers[0].Port)
	assert.Equal(t, 9999, servers[0].PID)
	assert.Equal(t, "dh serve", servers[0].Source)
	assert.Equal(t, "dh-serve", servers[0].Script)
}

func TestParseLsofOutputEmpty(t *testing.T) {
	servers := discovery.ParseLsofOutput("")
	assert.Empty(t, servers)
}

func TestClassifyCmdline(t *testing.T) {
	tests := []struct {
		name     string
		cmdline  string
		expected string
	}{
		{"dh serve command", "python -m dh serve --port 10000", "dh serve"},
		{"dh repl command", "python dh repl", "dh repl"},
		{"dh-serve hyphenated", "dh-serve --port 10000", "dh serve"},
		{"dh-repl hyphenated", "dh-repl", "dh repl"},
		{"java deephaven", "java -cp deephaven-server.jar io.deephaven.server.Main", "java"},
		{"plain java not matched", "java -jar something.jar", ""},
		{"python deephaven", "python -m deephaven_server run", "python"},
		{"unknown process", "nginx: worker process", ""},
		{"node process", "node /usr/lib/vscode/server.js", ""},
		{"empty cmdline", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := discovery.ClassifyCmdlineForTest(tt.cmdline)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeduplicateByPort(t *testing.T) {
	proc := []discovery.Server{
		{Port: 10000, PID: 1234, Source: "java"},
		{Port: 8080, PID: 5678, Source: "dh serve"},
	}
	docker := []discovery.Server{
		{Port: 10000, Source: "docker", ContainerID: "abc123"},
		{Port: 9090, Source: "docker", ContainerID: "def456"},
	}

	result := discovery.DeduplicateByPortForTest(proc, docker)
	require.Len(t, result, 3)

	// Process entry should win for port 10000
	assert.Equal(t, 10000, result[0].Port)
	assert.Equal(t, "java", result[0].Source)
	assert.Equal(t, 1234, result[0].PID)

	assert.Equal(t, 8080, result[1].Port)
	assert.Equal(t, 9090, result[2].Port)
}
