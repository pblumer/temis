package main

// Offline key-management CLI (WP-104): `temisd keys create|list|rotate|revoke`
// operates directly on the -keys-dir while the server is stopped, for lockout
// recovery (no usable admin key to reach POST /v1/keys). It shares the exact
// keystore + persistence path with the online lifecycle API, so a key minted
// here is accepted by the running server on its next start.

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/pblumer/temis/service"
)

const keysUsage = `Usage: temisd keys <command> [flags]

Manage the persistent scoped API keystore offline (server stopped), for lockout
recovery. The store directory defaults to $TEMIS_KEYS_DIR; override with -keys-dir.

Commands:
  create   mint a new key (prints the one-time secret)
  list     list keys without secrets
  rotate   issue a fresh secret for a key (invalidates the old one)
  revoke   revoke a key (marks it; never authenticates again)

Examples:
  temisd keys create -keys-dir ./keystore -scopes admin -owner recovery
  temisd keys create -keys-dir ./keystore -scopes evaluate,models:read -expires 2027-01-01T00:00:00Z
  temisd keys list   -keys-dir ./keystore
  temisd keys rotate -keys-dir ./keystore k_abc123
  temisd keys revoke -keys-dir ./keystore k_abc123
`

// runKeysCommand executes `temisd keys …` and returns a process exit code. args
// is os.Args after the "keys" token.
func runKeysCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, keysUsage)
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		return keysCreate(rest)
	case "list":
		return keysList(rest)
	case "rotate":
		return keysMutate(rest, "rotate")
	case "revoke":
		return keysMutate(rest, "revoke")
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(os.Stdout, keysUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "temisd keys: unknown command %q\n\n%s", sub, keysUsage)
		return 2
	}
}

// newKeysFlagSet builds a flag set carrying the shared -keys-dir flag.
func newKeysFlagSet(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet("keys "+name, flag.ContinueOnError)
	dir := fs.String("keys-dir", os.Getenv("TEMIS_KEYS_DIR"),
		"the persistent keystore directory (default $TEMIS_KEYS_DIR)")
	return fs, dir
}

func keysCreate(args []string) int {
	fs, dir := newKeysFlagSet("create")
	scopes := fs.String("scopes", "", "comma-separated scopes to grant (e.g. evaluate,models:read or evaluate:/orders/*)")
	owner := fs.String("owner", "", "free-form owner label for identity/audit")
	expires := fs.String("expires", "", "RFC3339 expiry (e.g. 2027-01-01T00:00:00Z); empty = never")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	scopeList := splitCSV(*scopes)
	if len(scopeList) == 0 {
		fmt.Fprintln(os.Stderr, "temisd keys create: -scopes is required")
		return 2
	}
	var expiry time.Time
	if strings.TrimSpace(*expires) != "" {
		t, err := time.Parse(time.RFC3339, *expires)
		if err != nil {
			fmt.Fprintf(os.Stderr, "temisd keys create: -expires not RFC3339: %v\n", err)
			return 2
		}
		expiry = t
	}
	admin, err := service.OpenKeyStore(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "temisd keys create: %v\n", err)
		return 1
	}
	kid, secret, err := admin.Create(scopeList, strings.TrimSpace(*owner), expiry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "temisd keys create: %v\n", err)
		return 1
	}
	// The secret is shown once here and never recoverable again.
	fmt.Printf("created key %s\n", kid)
	fmt.Printf("  bearer: %s.%s\n", kid, secret)
	fmt.Printf("  scopes: %s\n", strings.Join(scopeList, ", "))
	fmt.Println("  (store the bearer now — the secret is not recoverable)")
	return 0
}

func keysList(args []string) int {
	fs, dir := newKeysFlagSet("list")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	admin, err := service.OpenKeyStore(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "temisd keys list: %v\n", err)
		return 1
	}
	views := admin.List()
	if len(views) == 0 {
		fmt.Println("no keys")
		return 0
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KID\tSCOPES\tOWNER\tEXPIRES\tREVOKED\tMANAGED")
	for _, v := range views {
		scopes := make([]string, len(v.Scopes))
		for i, s := range v.Scopes {
			scopes[i] = string(s)
		}
		expires := v.ExpiresAt
		if expires == "" {
			expires = "-"
		}
		owner := v.Owner
		if owner == "" {
			owner = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%t\n", v.Kid, strings.Join(scopes, ","), owner, expires, v.Revoked, v.Managed)
	}
	_ = tw.Flush()
	return 0
}

// keysMutate handles rotate and revoke, which both take a single kid positional.
func keysMutate(args []string, action string) int {
	fs, dir := newKeysFlagSet(action)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	kid := fs.Arg(0)
	if kid == "" {
		fmt.Fprintf(os.Stderr, "temisd keys %s: a kid argument is required\n", action)
		return 2
	}
	admin, err := service.OpenKeyStore(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "temisd keys %s: %v\n", action, err)
		return 1
	}
	switch action {
	case "rotate":
		secret, err := admin.Rotate(kid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "temisd keys rotate: %v\n", err)
			return 1
		}
		fmt.Printf("rotated key %s\n", kid)
		fmt.Printf("  bearer: %s.%s\n", kid, secret)
		fmt.Println("  (the previous secret is now invalid)")
	case "revoke":
		if err := admin.Revoke(kid); err != nil {
			fmt.Fprintf(os.Stderr, "temisd keys revoke: %v\n", err)
			return 1
		}
		fmt.Printf("revoked key %s\n", kid)
	}
	return 0
}

// splitCSV splits a comma-separated list, trimming spaces and dropping empties.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
