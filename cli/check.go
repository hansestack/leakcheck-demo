package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/hansestack/hansestack-go/leakcheck"
	"github.com/spf13/cobra"
)

var (
	checkPasswordArg string
	checkQuiet       bool
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "check a single password against the breach corpus",
	Long: `check reads a password from stdin and reports whether it appears in a
known data-breach corpus.

The password is read from stdin by default so that it never appears in shell
history or in the process list. Only the first 5 characters of its SHA-1 hash
are sent to the API; the password itself never leaves this process.

Exit codes:
  0  password not found in any known breach
  1  password found in the breach corpus
  2  configuration or input error`,
	Example: `  echo -n 'hunter2' | leakcheck-demo check
  leakcheck-demo check --password 'hunter2'   # avoid: visible in shell history`,
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().StringVar(&checkPasswordArg, "password", "",
		"password to check (insecure: prefer stdin)")
	checkCmd.Flags().BoolVarP(&checkQuiet, "quiet", "q", false,
		"suppress output, communicate only via exit code")
}

// newClient builds a leak-check client from the loaded configuration.
//
// The defaults are the production-correct ones: a 500ms timeout and fail-open
// error handling. FailClose is exposed only so the demo can show both modes.
func newClient() *leakcheck.Client {
	opts := []leakcheck.Option{
		leakcheck.WithTimeout(appCfg.LeakCheck.Timeout),
		leakcheck.WithLogger(slog.Default()),
	}
	if appCfg.LeakCheck.FailClose {
		opts = append(opts, leakcheck.WithFailClose())
	}

	return leakcheck.NewClient(appCfg.LeakCheck.APIKey, opts...)
}

func runCheck(cmd *cobra.Command, _ []string) error {
	password, err := readPassword(cmd)
	if err != nil {
		return err
	}
	if password == "" {
		return errors.New("no password supplied: pipe one via stdin or use --password")
	}

	leaked, count, err := newClient().CheckPassword(context.Background(), password)
	if err != nil {
		// Only reachable with LEAKCHECK_FAIL_CLOSE=true. In a real login flow
		// you would log this and continue; a CLI may legitimately surface it.
		return fmt.Errorf("leak check failed: %w", err)
	}

	if leaked {
		if !checkQuiet {
			cmd.Printf("LEAKED: this password appeared in %d known data breaches\n", count)
		}
		// Bypass cobra's error handling to control the exit code precisely.
		os.Exit(exitLeaked)
	}

	if !checkQuiet {
		cmd.Println("OK: password not found in any known breach")
	}

	return nil
}

// readPassword takes the password from --password when set, otherwise from
// stdin. Reading from stdin keeps the secret out of shell history and out of
// the process list, where command-line arguments are world-readable.
func readPassword(cmd *cobra.Command) (string, error) {
	if checkPasswordArg != "" {
		return checkPasswordArg, nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("could not inspect stdin: %w", err)
	}

	// Refuse to block forever when stdin is an interactive terminal.
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", errors.New("no password on stdin: pipe one in, e.g. echo -n 'pw' | leakcheck-demo check")
	}

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("could not read password from stdin: %w", err)
	}

	// Strip only the trailing newline; a password may legitimately contain
	// leading or trailing spaces.
	return strings.TrimRight(line, "\r\n"), nil
}
