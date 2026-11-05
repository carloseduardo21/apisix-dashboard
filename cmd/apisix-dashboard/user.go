package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

func userCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("missing subcommand: add, passwd, reset-mfa, unlock or list")
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("user "+sub, flag.ContinueOnError)
	username := fs.String("u", "", "username")
	role := fs.String("r", auth.RoleViewer, "role: viewer, operator or admin")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// CLI actions are audited too, but only to the database: stdout is the
	// operator's terminal.
	st, err := open(nil)
	if err != nil {
		return err
	}
	defer st.store.Close()
	ctx := context.Background()

	if sub == "list" {
		users, err := st.store.ListUsers(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%-24s %-9s %-4s %-9s %s\n", "USERNAME", "ROLE", "MFA", "STATUS", "LAST LOGIN")
		for _, u := range users {
			status := "active"
			switch {
			case u.Disabled:
				status = "disabled"
			case time.Now().Before(u.LockedUntil):
				status = "locked"
			}
			mfa := "no"
			if u.TOTPSecret != nil {
				mfa = "yes"
			}
			last := "-"
			if !u.LastLoginAt.IsZero() {
				last = u.LastLoginAt.Format(time.RFC3339)
			}
			fmt.Printf("%-24s %-9s %-4s %-9s %s\n", u.Username, u.Role, mfa, status, last)
		}
		return nil
	}

	if *username == "" {
		return errors.New("set the username with -u")
	}
	record := func(action, detail string) {
		st.audit.Record(ctx, store.AuditEvent{
			Actor: cliActor(), Action: action, Resource: "dashboard_users",
			ResourceID: *username, Outcome: audit.Success, Detail: detail,
		})
	}

	switch sub {
	case "add":
		pw, err := readPassword()
		if err != nil {
			return err
		}
		if _, err := st.auth.CreateUser(ctx, *username, *role, pw, false); err != nil {
			return err
		}
		record("user.create", "role="+*role)
		fmt.Printf("user %q created (%s). MFA will be set up at the first login.\n", *username, *role)
		return nil
	}

	u, err := st.store.GetUserByUsername(ctx, *username)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("user %q not found", *username)
	}
	if err != nil {
		return err
	}
	switch sub {
	case "passwd":
		pw, err := readPassword()
		if err != nil {
			return err
		}
		if err := st.auth.SetPassword(ctx, u, pw, true); err != nil {
			return err
		}
		record("user.password_reset", "")
		fmt.Println("password set; the user must change it at the next login.")
	case "reset-mfa":
		u.TOTPSecret = nil
		u.TOTPLastStep = 0
		u.UpdatedAt = time.Now()
		if err := st.store.UpdateUser(ctx, u); err != nil {
			return err
		}
		record("user.mfa_reset", "")
		fmt.Println("MFA removed; it will be set up again at the next login.")
	case "unlock":
		u.FailedAttempts = 0
		u.LockedUntil = time.Time{}
		u.UpdatedAt = time.Now()
		if err := st.store.UpdateUser(ctx, u); err != nil {
			return err
		}
		record("user.unlock", "")
		fmt.Println("user unlocked.")
	default:
		return fmt.Errorf("unknown subcommand: %s", sub)
	}
	// Any credential change ends existing sessions.
	return st.store.DeleteUserSessions(ctx, u.ID, "")
}

func cliActor() string {
	name := "unknown"
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	return "cli:" + name
}

// readPassword prompts twice on a terminal, or reads one line from stdin
// when piped (for automation).
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no password given on stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprintf(os.Stderr, "Password (min. %d characters, letters and digits): ", auth.MinPasswordLength)
	a, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Confirm the password: ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", errors.New("the passwords do not match")
	}
	return string(a), nil
}
