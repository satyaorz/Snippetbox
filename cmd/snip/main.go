// Command snip is a command-line client for Snippetbox.
//
//	snip login                    log in and save an API token
//	snip [create] [flags] [FILE]  create a snippet from FILE or stdin, print its URL
//	snip get SLUG                 print a snippet's content
//	snip ls                       list your snippets
//	snip search QUERY             search public snippets
//	snip rm SLUG                  delete one of your snippets
//	snip whoami                   show who you're logged in as
//	snip logout                   revoke the saved token
//
// Every command accepts -server URL (or SNIP_SERVER) and -cacert FILE (or
// SNIP_CACERT, e.g. tls/cert.pem to trust the development certificate).
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/satyaorz/snippetbox/internal/lang"

	"golang.org/x/term"
)

const defaultServer = "https://localhost:4000"

// env is everything the CLI touches outside its arguments, so tests can
// provide their own.
type env struct {
	stdin        io.Reader
	stdout       io.Writer
	stderr       io.Writer
	getenv       func(string) string
	stdinIsTTY   bool
	readPassword func() (string, error)
	hostname     string
}

func main() {
	hostname, _ := os.Hostname()

	e := env{
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		getenv:     os.Getenv,
		stdinIsTTY: term.IsTerminal(int(os.Stdin.Fd())),
		readPassword: func() (string, error) {
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			return string(b), err
		},
		hostname: hostname,
	}

	err := run(os.Args[1:], e)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "snip:", err)
		}
		os.Exit(1)
	}
}

const usage = `usage: snip <command> [flags] [args]

commands:
  login                      log in and save an API token
  create [flags] [FILE]      create a snippet from FILE or stdin (the default command)
  get SLUG                   print a snippet's content
  ls                         list your snippets
  search QUERY               search public snippets
  rm SLUG                    delete one of your snippets
  whoami                     show who you're logged in as
  logout                     revoke the saved token

run "snip <command> -h" for a command's flags`

var commands = map[string]func(*session, []string) error{
	"login":  (*session).login,
	"logout": (*session).logout,
	"whoami": (*session).whoami,
	"create": (*session).create,
	"get":    (*session).get,
	"ls":     (*session).list,
	"search": (*session).search,
	"rm":     (*session).remove,
}

func run(args []string, e env) error {
	name := "create"
	if len(args) > 0 {
		if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
			fmt.Fprintln(e.stdout, usage)
			return nil
		}
		if _, ok := commands[args[0]]; ok {
			name, args = args[0], args[1:]
		}
	}

	s := &session{env: e, name: name}
	return commands[name](s, args)
}

// session holds the state for running one command.
type session struct {
	env
	name       string
	server     string
	caCert     string
	configPath string
	config     savedConfig
	ctx        context.Context
}

// flags returns a FlagSet for the command with the global -server and
// -cacert flags already registered.
func (s *session) flags() *flag.FlagSet {
	fs := flag.NewFlagSet("snip "+s.name, flag.ContinueOnError)
	fs.SetOutput(s.stderr)
	fs.StringVar(&s.server, "server", "", "Snippetbox server URL (default $SNIP_SERVER, the saved server, or "+defaultServer+")")
	fs.StringVar(&s.caCert, "cacert", s.getenv("SNIP_CACERT"), "extra CA certificate to trust, e.g. tls/cert.pem")
	return fs
}

// client loads the saved config and returns an API client for the chosen
// server. If requireLogin is set, it fails early when there's no token.
func (s *session) client(requireLogin bool) (*client, error) {
	var err error
	s.ctx = context.Background()

	s.configPath, err = configPath(s.getenv)
	if err != nil {
		return nil, err
	}
	s.config, err = loadConfig(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.configPath, err)
	}

	// Server: -server flag, then $SNIP_SERVER, then the saved server.
	server := firstNonEmpty(s.server, s.getenv("SNIP_SERVER"), s.config.Server, defaultServer)

	// Only send the saved token to the server it was issued by.
	token := s.getenv("SNIP_TOKEN")
	if token == "" && strings.TrimRight(s.config.Server, "/") == strings.TrimRight(server, "/") {
		token = s.config.Token
	}
	if requireLogin && token == "" {
		return nil, fmt.Errorf("not logged in to %s: run `snip login` first", server)
	}

	s.server = server
	return newClient(server, token, s.caCert)
}

// parseArgs parses flags that may appear before or after the positional
// arguments (Go's flag package stops at the first non-flag), so both
// `snip -v public main.go` and `snip main.go -v public` work. Everything after
// a "--" is positional. Afterwards fs.Args() holds the positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) error {
	var positional []string

	for len(args) > 0 {
		if args[0] == "--" {
			positional = append(positional, args[1:]...)
			break
		}

		if err := fs.Parse(args); err != nil {
			return err
		}

		// fs.Parse stops at the first non-flag argument, or just after a "--".
		consumed := len(args) - fs.NArg()
		if consumed > 0 && args[consumed-1] == "--" {
			positional = append(positional, fs.Args()...)
			break
		}

		args = fs.Args()
		if len(args) > 0 {
			positional = append(positional, args[0])
			args = args[1:]
		}
	}

	return fs.Parse(append([]string{"--"}, positional...))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// loginHint turns a 401 into a friendlier message.
func loginHint(err error) error {
	if isUnauthorized(err) {
		return fmt.Errorf("%w: your token may have expired or been revoked, run `snip login`", err)
	}
	return err
}

func (s *session) login(args []string) error {
	fs := s.flags()
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	c, err := s.client(false)
	if err != nil {
		return err
	}

	in := bufio.NewReader(s.stdin)

	fmt.Fprint(s.stderr, "Email: ")
	email, err := in.ReadString('\n')
	if err != nil && email == "" {
		return errors.New("no email entered")
	}
	email = strings.TrimSpace(email)

	fmt.Fprint(s.stderr, "Password: ")
	var password string
	if s.stdinIsTTY {
		password, err = s.readPassword()
		fmt.Fprintln(s.stderr)
	} else {
		// Piped input (e.g. from a password manager): read the next line.
		password, err = in.ReadString('\n')
		if err == io.EOF && password != "" {
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	password = strings.TrimRight(password, "\r\n")

	token, expires, err := c.createToken(s.ctx, email, password, "snip on "+s.hostname)
	if err != nil {
		return err
	}

	err = saveConfig(s.configPath, savedConfig{Server: s.server, Token: token})
	if err != nil {
		return err
	}

	fmt.Fprintf(s.stdout, "Logged in to %s. Token saved to %s (expires %s).\n", s.server, s.configPath, expires.Local().Format(time.DateOnly))
	return nil
}

func (s *session) logout(args []string) error {
	fs := s.flags()
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	c, err := s.client(true)
	if err != nil {
		return err
	}

	// Revoke the token on the server, but forget it locally either way.
	err = c.revokeToken(s.ctx)
	if err != nil && !isUnauthorized(err) {
		return err
	}

	s.config.Token = ""
	if err := saveConfig(s.configPath, s.config); err != nil {
		return err
	}

	fmt.Fprintln(s.stdout, "Logged out.")
	return nil
}

func (s *session) whoami(args []string) error {
	fs := s.flags()
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	c, err := s.client(true)
	if err != nil {
		return err
	}

	name, email, err := c.me(s.ctx)
	if err != nil {
		return loginHint(err)
	}

	fmt.Fprintf(s.stdout, "%s <%s> on %s\n", name, email, s.server)
	return nil
}

func (s *session) create(args []string) error {
	fs := s.flags()
	title := fs.String("t", "", "title (default: the file name)")
	language := fs.String("l", "", "language, e.g. go or python (default: detected from the file name)")
	visibility := fs.String("v", "unlisted", "visibility: public, unlisted or private")
	expires := fs.Int("e", 7, "delete after this many days: 1, 7 or 365")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: snip [create] [flags] [FILE]\n\nReads FILE, or stdin if FILE is omitted or -.")
		fs.PrintDefaults()
	}
	if err := parseArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("create takes at most one file")
	}

	var content []byte
	var err error
	file := fs.Arg(0)

	if file == "" || file == "-" {
		if s.stdinIsTTY {
			return errors.New("nothing to upload: pass a file name or pipe something in, e.g. `snip < main.go`")
		}
		content, err = io.ReadAll(s.stdin)
	} else {
		content, err = os.ReadFile(file)
	}
	if err != nil {
		return err
	}

	if *title == "" {
		*title = "Untitled"
		if file != "" && file != "-" {
			*title = filepath.Base(file)
		}
	}
	if *language == "" && file != "" {
		*language = lang.Detect(file)
	}

	c, err := s.client(true)
	if err != nil {
		return err
	}

	created, err := c.createSnippet(s.ctx, newSnippet{
		Title:         *title,
		Content:       string(content),
		Language:      *language,
		Visibility:    *visibility,
		ExpiresInDays: *expires,
	})
	if err != nil {
		return loginHint(err)
	}

	fmt.Fprintln(s.stdout, created.URL)
	return nil
}

func (s *session) get(args []string) error {
	fs := s.flags()
	if err := parseArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: snip get SLUG")
	}

	c, err := s.client(false)
	if err != nil {
		return err
	}

	sn, err := c.getSnippet(s.ctx, slugFromArg(fs.Arg(0)))
	if err != nil {
		return loginHint(err)
	}

	io.WriteString(s.stdout, sn.Content)
	if !strings.HasSuffix(sn.Content, "\n") {
		fmt.Fprintln(s.stdout)
	}
	return nil
}

// slugFromArg accepts either a slug or a full snippet URL.
func slugFromArg(arg string) string {
	arg = strings.TrimRight(arg, "/")
	if i := strings.LastIndex(arg, "/"); i >= 0 {
		return arg[i+1:]
	}
	return arg
}

func (s *session) list(args []string) error {
	fs := s.flags()
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	c, err := s.client(true)
	if err != nil {
		return err
	}

	snippets, err := c.mySnippets(s.ctx)
	if err != nil {
		return loginHint(err)
	}

	if len(snippets) == 0 {
		fmt.Fprintln(s.stdout, "You don't have any snippets yet.")
		return nil
	}
	s.printTable(snippets, true)
	return nil
}

func (s *session) search(args []string) error {
	fs := s.flags()
	if err := parseArgs(fs, args); err != nil {
		return err
	}
	query := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(query) == "" {
		return errors.New("usage: snip search QUERY")
	}

	c, err := s.client(false)
	if err != nil {
		return err
	}

	snippets, total, err := c.searchSnippets(s.ctx, query)
	if err != nil {
		return loginHint(err)
	}

	if total == 0 {
		fmt.Fprintln(s.stdout, "No snippets matched.")
		return nil
	}
	s.printTable(snippets, false)
	if total > len(snippets) {
		fmt.Fprintf(s.stdout, "(showing %d of %d)\n", len(snippets), total)
	}
	return nil
}

func (s *session) printTable(snippets []snippet, mine bool) {
	tw := tabwriter.NewWriter(s.stdout, 0, 4, 2, ' ', 0)
	if mine {
		fmt.Fprintln(tw, "SLUG\tVISIBILITY\tEXPIRES\tTITLE")
	} else {
		fmt.Fprintln(tw, "SLUG\tAUTHOR\tCREATED\tTITLE")
	}
	for _, sn := range snippets {
		if mine {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", sn.Slug, sn.Visibility, sn.Expires.Local().Format(time.DateOnly), sn.Title)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", sn.Slug, firstNonEmpty(sn.Author, "anonymous"), sn.Created.Local().Format(time.DateOnly), sn.Title)
		}
	}
	tw.Flush()
}

func (s *session) remove(args []string) error {
	fs := s.flags()
	if err := parseArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: snip rm SLUG")
	}

	c, err := s.client(true)
	if err != nil {
		return err
	}

	slug := slugFromArg(fs.Arg(0))
	if err := c.deleteSnippet(s.ctx, slug); err != nil {
		return loginHint(err)
	}

	fmt.Fprintln(s.stdout, "Deleted", slug)
	return nil
}
