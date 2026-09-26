// Package auth authenticates API requests against the host's local PAM
// stack (basic auth over the "go-dhcpd" PAM service) and authorizes the
// resulting identity against a configured allow-list of users/groups.
package auth

import (
	"fmt"
	"os/user"

	"github.com/msteinert/pam/v2"
)

// ServiceName is the PAM service the daemon authenticates against. An
// /etc/pam.d/go-dhcpd file must exist (see pam.d/ in the repo) or PAM will
// fall back to the system's "other" stack, which typically denies everything.
const ServiceName = "go-dhcpd"

// Authenticate verifies username/password against the local PAM stack and
// confirms the account is valid (not locked/expired).
func Authenticate(username, password string) error {
	t, err := pam.StartFunc(ServiceName, username, func(s pam.Style, _ string) (string, error) {
		switch s {
case pam.PromptEchoOff:
			return password, nil
		case pam.PromptEchoOn:
			return username, nil
		default:
			return "", nil
		}
	})
	if err != nil {
		return fmt.Errorf("pam: failed to start transaction: %w", err)
	}
	defer t.End()

	if err := t.Authenticate(0); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	if err := t.AcctMgmt(0); err != nil {
		return fmt.Errorf("account is not valid: %w", err)
	}
	return nil
}

// Authorize reports whether username may access authenticated endpoints:
// root is always permitted, as is any user explicitly named in
// allowedUsers, or a member of any group in allowedGroups. If both lists
// are empty, only root is authorized.
func Authorize(username string, allowedUsers, allowedGroups []string) error {
	if username == "root" {
		return nil
	}
	for _, u := range allowedUsers {
		if u == username {
			return nil
		}
	}

	if len(allowedGroups) > 0 {
		usr, err := user.Lookup(username)
		if err != nil {
			return fmt.Errorf("failed to look up user %q: %w", username, err)
		}
		gids, err := usr.GroupIds()
		if err != nil {
			return fmt.Errorf("failed to look up groups for %q: %w", username, err)
		}
		for _, gid := range gids {
			group, err := user.LookupGroupId(gid)
			if err != nil {
				continue
			}
			for _, allowed := range allowedGroups {
				if group.Name == allowed {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("user %q is not authorized", username)
}
