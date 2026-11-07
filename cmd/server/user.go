package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/service"
)

const userUsage = `Manage users who can sign in to the API.

Usage:
  metareel user list
  metareel user create [-role admin|reader] <username>   (asks for the password)
  metareel user passwd <username>                        (asks; signs the user out everywhere)
  metareel user role <username> <admin|reader>
  metareel user disable <username>                       (also signs the user out)
  metareel user enable <username>
  metareel user logout <username>                        (ends every session)

Passwords are read from the terminal, or from the first line of stdin when it
is not a terminal (e.g. docker exec -i metareel /app/metareel user passwd alice < pw.txt).
`

// runUserCommand handles "metareel user …" against the configured database.
func runUserCommand(cfg *config.Config, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, userUsage)
		return nil
	}
	ctx := context.Background()
	db, err := repository.Open(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	// Keep goose's progress lines (standard logger) out of the CLI output.
	stdlog.SetOutput(io.Discard)
	if err := repository.Migrate(db, cfg.Database.MigrationsDir); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := repository.NewUserRepository(db)
	sessions := service.NewSessionService(log, repo,
		service.SessionConfig{TTL: cfg.Auth.SessionTTL, IdleTimeout: cfg.Auth.SessionIdleTimeout})
	users := service.NewUserService(log, repo, sessions)

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "list":
		list, err := users.List(ctx)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tUSERNAME\tROLE\tSTATUS\tLAST LOGIN")
		for _, u := range list {
			status, last := "active", "never"
			if u.Disabled {
				status = "disabled"
			}
			if u.LastLoginAt != nil {
				last = u.LastLoginAt.UTC().Format(time.DateTime)
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", u.ID, u.Username, u.Role, status, last)
		}
		return w.Flush()

	case "create":
		fs := flag.NewFlagSet("user create", flag.ContinueOnError)
		role := fs.String("role", service.RoleReader, "admin or reader")
		username, err := parseWithName(fs, rest)
		if err != nil {
			return err
		}
		pw, err := readNewPassword(stdin, stdout)
		if err != nil {
			return err
		}
		u, err := users.Create(ctx, username, pw, *role)
		if err != nil {
			return userCmdErr(err)
		}
		fmt.Fprintf(stdout, "created %s (%s)\n", u.Username, u.Role)
		return nil

	case "passwd":
		username, err := oneArg(cmd, rest)
		if err != nil {
			return err
		}
		pw, err := readNewPassword(stdin, stdout)
		if err != nil {
			return err
		}
		if err := users.SetPassword(ctx, username, pw); err != nil {
			return userCmdErr(err)
		}
		fmt.Fprintf(stdout, "password changed for %s; existing sessions ended\n", username)
		return nil

	case "role":
		if len(rest) != 2 {
			return errors.New("usage: metareel user role <username> <admin|reader>")
		}
		if err := users.SetRole(ctx, rest[0], rest[1]); err != nil {
			return userCmdErr(err)
		}
		fmt.Fprintf(stdout, "%s is now %s\n", rest[0], rest[1])
		return nil

	case "disable", "enable":
		username, err := oneArg(cmd, rest)
		if err != nil {
			return err
		}
		if err := users.SetDisabled(ctx, username, cmd == "disable"); err != nil {
			return userCmdErr(err)
		}
		fmt.Fprintf(stdout, "%sd %s\n", cmd, username)
		return nil

	case "logout":
		username, err := oneArg(cmd, rest)
		if err != nil {
			return err
		}
		n, err := sessions.RevokeUser(ctx, username)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "ended %d session(s) of %s\n", n, username)
		return nil
	}
	return fmt.Errorf("unknown user command %q\n\n%s", cmd, userUsage)
}

// parseWithName parses flags that may come before or after the username.
func parseWithName(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() == 0 {
		return "", fmt.Errorf("usage: metareel %s [flags] <username>", fs.Name())
	}
	name := fs.Arg(0)
	if err := fs.Parse(fs.Args()[1:]); err != nil {
		return "", err
	}
	if fs.NArg() > 0 {
		return "", fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return name, nil
}

func oneArg(cmd string, args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: metareel user %s <username>", cmd)
	}
	return args[0], nil
}

func userCmdErr(err error) error {
	var vErr *service.ValidationError
	if errors.As(err, &vErr) {
		return errors.New(vErr.Message)
	}
	return err
}

// readNewPassword asks twice on a terminal, or reads one line from stdin.
func readNewPassword(stdin io.Reader, stdout io.Writer) (string, error) {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(stdout, "Password: ")
		a, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stdout)
		if err != nil {
			return "", err
		}
		fmt.Fprint(stdout, "Repeat password: ")
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(stdout)
		if err != nil {
			return "", err
		}
		if string(a) != string(b) {
			return "", errors.New("passwords do not match")
		}
		return string(a), nil
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", errors.New("no password on stdin")
	}
	return line, nil
}
