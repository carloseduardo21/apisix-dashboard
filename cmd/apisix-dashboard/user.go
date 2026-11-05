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
		return errors.New("subcomando ausente: add, passwd, reset-mfa, unlock ou list")
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("user "+sub, flag.ContinueOnError)
	username := fs.String("u", "", "nome do usuário")
	role := fs.String("r", auth.RoleViewer, "perfil: viewer, operator ou admin")
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
		fmt.Printf("%-24s %-9s %-4s %-9s %s\n", "USUÁRIO", "PERFIL", "MFA", "STATUS", "ÚLTIMO LOGIN")
		for _, u := range users {
			status := "ativo"
			switch {
			case u.Disabled:
				status = "inativo"
			case time.Now().Before(u.LockedUntil):
				status = "bloqueado"
			}
			mfa := "não"
			if u.TOTPSecret != nil {
				mfa = "sim"
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
		return errors.New("informe o usuário com -u")
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
		fmt.Printf("usuário %q criado (%s). O MFA será configurado no primeiro login.\n", *username, *role)
		return nil
	}

	u, err := st.store.GetUserByUsername(ctx, *username)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("usuário %q não encontrado", *username)
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
		fmt.Println("senha definida; o usuário deverá trocá-la no próximo login.")
	case "reset-mfa":
		u.TOTPSecret = nil
		u.TOTPLastStep = 0
		u.UpdatedAt = time.Now()
		if err := st.store.UpdateUser(ctx, u); err != nil {
			return err
		}
		record("user.mfa_reset", "")
		fmt.Println("MFA removido; será configurado novamente no próximo login.")
	case "unlock":
		u.FailedAttempts = 0
		u.LockedUntil = time.Time{}
		u.UpdatedAt = time.Now()
		if err := st.store.UpdateUser(ctx, u); err != nil {
			return err
		}
		record("user.unlock", "")
		fmt.Println("usuário desbloqueado.")
	default:
		return fmt.Errorf("subcomando desconhecido: %s", sub)
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
			return "", errors.New("senha não informada no stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprintf(os.Stderr, "Senha (mín. %d caracteres, letras e números): ", auth.MinPasswordLength)
	a, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Confirme a senha: ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", errors.New("as senhas não conferem")
	}
	return string(a), nil
}
