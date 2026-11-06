// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/boggycreek/lokol/liblokol/config"
)

// ProfileRulesStage enforces declarative workload profile constraints adhering to ADR-0023 and ADR-0027.
type ProfileRulesStage struct {
	rules config.RegulatorRules
}

// NewProfileRulesStage creates a new regulator stage enforcing profile-specific restrictions.
func NewProfileRulesStage(rules config.RegulatorRules) *ProfileRulesStage {
	return &ProfileRulesStage{rules: rules}
}

func (s *ProfileRulesStage) Name() string {
	return "profile_rules"
}

// Evaluate checks candidate actions against active profile rules.
func (s *ProfileRulesStage) Evaluate(ctx context.Context, action ActionCandidate, workDir string) PermissionResult {
	actionName := strings.ToLower(strings.TrimSpace(action.Name))

	// 1. Read-only profile constraint: block mutating actions
	if s.rules.ReadOnly {
		if actionName == "write_file" || actionName == "replace_file" {
			return PermissionResult{
				Status:      StatusBlocked,
				Reason:      fmt.Sprintf("Profile is configured in read-only mode: mutation action %q is prohibited", action.Name),
				RiskLevel:   RiskLevelHigh,
				Target:      action.Name,
				Remediation: "The active profile restricts operations to read-only analysis. Use read_window, read_outline, or search_code to inspect workspace content.",
			}
		}
	}

	// 2. Explicitly blocked tool actions
	for _, blocked := range s.rules.BlockedActions {
		if strings.EqualFold(actionName, strings.TrimSpace(blocked)) {
			return PermissionResult{
				Status:      StatusBlocked,
				Reason:      fmt.Sprintf("Action %q is blocked by active profile policy", action.Name),
				RiskLevel:   RiskLevelHigh,
				Target:      action.Name,
				Remediation: fmt.Sprintf("Action %q is disallowed by profile configuration. Use an authorized tool or adjust your workload profile.", action.Name),
			}
		}
	}

	// 3. Disallowed commands (for exec_bash)
	if actionName == "exec_bash" && len(s.rules.DisallowedCommands) > 0 {
		cmd := strings.TrimSpace(action.Command)
		for _, disallowed := range s.rules.DisallowedCommands {
			d := strings.TrimSpace(disallowed)
			if d == "" {
				continue
			}
			if matchesDisallowedCommand(cmd, d) {
				return PermissionResult{
					Status:      StatusBlocked,
					Reason:      fmt.Sprintf("Shell command contains prohibited token %q per profile policy", d),
					RiskLevel:   RiskLevelHigh,
					Target:      cmd,
					Remediation: fmt.Sprintf("Command %q matches disallowed command pattern %q. Reformulate using authorized tools.", cmd, d),
				}
			}
		}
	}

	// 4. Disallowed path patterns
	if len(s.rules.DisallowedPaths) > 0 {
		targetPath := action.Path
		if targetPath == "" {
			targetPath = ResolveActionPath(action)
		}
		if targetPath != "" {
			baseName := filepath.Base(targetPath)
			cleanPath := filepath.Clean(targetPath)
			for _, pat := range s.rules.DisallowedPaths {
				pat = strings.TrimSpace(pat)
				if pat == "" {
					continue
				}
				matchedBase, _ := filepath.Match(pat, baseName)
				matchedPath, _ := filepath.Match(pat, cleanPath)
				if matchedBase || matchedPath || strings.Contains(cleanPath, pat) {
					return PermissionResult{
						Status:      StatusBlocked,
						Reason:      fmt.Sprintf("Target path %q is prohibited by profile path restriction %q", targetPath, pat),
						RiskLevel:   RiskLevelHigh,
						Target:      targetPath,
						Remediation: fmt.Sprintf("Access to path %q matching rule %q is prohibited in this profile.", targetPath, pat),
					}
				}
			}
		}
	}

	return PermissionResult{
		Status:    StatusAllowed,
		RiskLevel: RiskLevelNone,
	}
}

func matchesDisallowedCommand(fullCommand, prohibited string) bool {
	fields := strings.Fields(fullCommand)
	for _, f := range fields {
		if strings.EqualFold(f, prohibited) {
			return true
		}
		// Match base name if prohibited token is executable name (e.g. /bin/rm -> rm)
		if strings.EqualFold(filepath.Base(f), prohibited) {
			return true
		}
	}
	return false
}
