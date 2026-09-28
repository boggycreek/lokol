// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package regulator

import (
	"regexp"
	"strings"
)

// RiskLevel defines the severity level of a detected command risk.
type RiskLevel string

const (
	RiskLevelNone     RiskLevel = "none"
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// ShellRisk reports the findings from deterministic static analysis of a shell command.
type ShellRisk struct {
	Level   RiskLevel
	Reason  string
	Pattern string
}

type dangerPattern struct {
	regex  *regexp.Regexp
	level  RiskLevel
	reason string
}

var destructiveSystemRoots = []string{
	"/", "/*",
	"~", "~/*", "$HOME", "$HOME/*",
	"/etc", "/etc/*",
	"/var", "/var/*",
	"/usr", "/usr/*",
	"/bin", "/bin/*",
	"/sbin", "/sbin/*",
	"/lib", "/lib/*",
	"/lib64", "/lib64/*",
	"/opt", "/opt/*",
	"/boot", "/boot/*",
	"/sys", "/sys/*",
	"/proc", "/proc/*",
	"/dev", "/dev/*",
	"/root", "/root/*",
}

var defaultDangerPatterns = []dangerPattern{
	// Critical: Raw device format, partition table manipulation, raw block writes, fork bombs
	{
		regex:  regexp.MustCompile(`(?i)\b(?:mkfs|fdisk|parted|sfdisk|gdisk)\b`),
		level:  RiskLevelCritical,
		reason: "Direct block device formatting or partition table manipulation",
	},
	{
		regex:  regexp.MustCompile(`(?i)\bdd\s+.*of=/dev/(?:sd[a-z]|nvme[0-9]|vd[a-z]|hd[a-z]|loop[0-9]|mapper)`),
		level:  RiskLevelCritical,
		reason: "Direct raw block device write via dd",
	},
	{
		regex:  regexp.MustCompile(`:\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
		level:  RiskLevelCritical,
		reason: "Fork bomb resource exhaustion attack",
	},

	// High: Privilege escalation
	{
		regex:  regexp.MustCompile(`(?i)\b(?:sudo|doas|pkexec|su(?:\s+-|\s+root|\s*$))\b`),
		level:  RiskLevelHigh,
		reason: "Privilege escalation attempt requiring superuser credentials",
	},

	// High: Process substitution execution (lokol-gml.3)
	{
		regex:  regexp.MustCompile(`(?i)(?:bash|sh|zsh|dash)\s+<\([^)]+\)`),
		level:  RiskLevelHigh,
		reason: "Process substitution executing unverified stream directly in shell interpreter",
	},

	// High: Remote execution or encoded decode pipelines (lokol-gml.3)
	{
		regex:  regexp.MustCompile(`(?i)\b(?:curl|wget|fetch|lynx)\s+.*\|\s*(?:/usr/bin/|/bin/)?(?:env\s+)?(?:bash|sh|zsh|dash|python|perl|ruby)\b`),
		level:  RiskLevelHigh,
		reason: "Piping unverified remote web content directly into shell interpreter",
	},
	{
		regex:  regexp.MustCompile(`(?i)\bbase64\s+(?:-d|--decode)\s*\|\s*(?:/usr/bin/|/bin/)?(?:env\s+)?(?:bash|sh|zsh|dash|python|perl|ruby)\b`),
		level:  RiskLevelHigh,
		reason: "Piping decoded base64 payload directly into shell interpreter",
	},
	{
		regex:  regexp.MustCompile(`(?i)\b(?:export\s+)?LD_PRELOAD=`),
		level:  RiskLevelHigh,
		reason: "Dynamic library preload injection attack (LD_PRELOAD)",
	},

	// High: Credential access and exfiltration (lokol-gml.3: generalized across tools)
	{
		regex:  regexp.MustCompile(`(?i)\b(?:cat|head|tail|less|more|cp|mv|tar|zip|xxd|hexdump|strings|base64|grep|awk|sed|scp|rsync|python|ruby|perl|node)\s+.*(?:~|\$HOME|\/home\/[^\/]+|\/root)\/\.(?:ssh|aws|gnupg|docker\/config\.json|config\/gh)\b`),
		level:  RiskLevelHigh,
		reason: "Attempt to access host authentication credentials or private keys",
	},
	{
		regex:  regexp.MustCompile(`(?i)\b.*(?:~|\$HOME|\/home\/[^\/]+|\/root)\/\.(?:ssh\/id_|aws\/credentials|gnupg\/secring)\b`),
		level:  RiskLevelHigh,
		reason: "Direct access targeting sensitive private keys or authentication tokens",
	},
	{
		regex:  regexp.MustCompile(`(?i)\b(?:cat|head|tail|less|more|cp|mv|grep|awk|sed|tar|xxd)\s+.*\/etc\/(?:shadow|sudoers|master\.passwd|passwd)\b`),
		level:  RiskLevelHigh,
		reason: "Attempt to access sensitive host security and password databases",
	},
	{
		regex:  regexp.MustCompile(`(?i)\bgetent\s+(?:passwd|shadow)\b`),
		level:  RiskLevelHigh,
		reason: "Attempt to dump host user and authentication databases",
	},

	// High: Writing directly into system service or cron scheduler configs
	{
		regex:  regexp.MustCompile(`(?i)(?:>|>>)\s*\/etc\/(?:cron|systemd|init\.d|pam\.d)`),
		level:  RiskLevelHigh,
		reason: "Attempt to write directly into system service or cron scheduler configurations",
	},

	// Medium: Modifying global system directories
	{
		regex:  regexp.MustCompile(`(?i)\b(?:chmod|chown|chgrp)\s+.*(?:\s|/)\/(?:etc|bin|sbin|usr|lib|boot|sys|proc)\b`),
		level:  RiskLevelMedium,
		reason: "Permission tampering on global system directories",
	},
}

// InspectShellRisk analyzes a shell command for dangerous operations, regex evasions, and host system risks (lokol-gml.3).
func InspectShellRisk(workDir string, command string) ShellRisk {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ShellRisk{Level: RiskLevelNone}
	}

	// 1. Destructive Recursive Removal Analysis (Normalized for split flags & system trees)
	if rmRisk := inspectDestructiveRemoval(trimmed); rmRisk.Level != RiskLevelNone {
		return rmRisk
	}

	// 2. Pattern-based threat analysis
	for _, p := range defaultDangerPatterns {
		if p.regex.MatchString(trimmed) {
			return ShellRisk{
				Level:   p.level,
				Reason:  p.reason,
				Pattern: p.regex.String(),
			}
		}
	}

	return ShellRisk{Level: RiskLevelNone}
}

// inspectDestructiveRemoval detects destructive recursive file deletions regardless of flag order or splitting (lokol-gml.3).
func inspectDestructiveRemoval(cmd string) ShellRisk {
	// Split compound commands by operators (;, &&, ||, |, \n)
	subCmds := strings.FieldsFunc(cmd, func(r rune) bool {
		return r == ';' || r == '&' || r == '|' || r == '\n'
	})

	for _, sub := range subCmds {
		tokens := strings.Fields(strings.TrimSpace(sub))
		if len(tokens) < 2 {
			continue
		}

		baseCmd := tokens[0]
		if baseCmd != "rm" && !strings.HasSuffix(baseCmd, "/rm") {
			continue
		}

		hasRecursive := false
		noPreserveRoot := false
		var targets []string

		for _, token := range tokens[1:] {
			if token == "--no-preserve-root" {
				noPreserveRoot = true
				continue
			}
			if token == "-r" || token == "-R" || token == "--recursive" {
				hasRecursive = true
				continue
			}
			if token == "-f" || token == "--force" {
				continue
			}
			if strings.HasPrefix(token, "-") && !strings.HasPrefix(token, "--") {
				// Combined flags like -rf, -fr, -rfi, etc.
				flagStr := strings.TrimPrefix(token, "-")
				if strings.ContainsAny(flagStr, "rR") {
					hasRecursive = true
				}
				continue
			}
			// Non-flag arguments are targets
			targets = append(targets, token)
		}

		if hasRecursive {
			for _, target := range targets {
				cleaned := strings.TrimRight(target, "/")
				if cleaned == "" {
					cleaned = "/"
				}

				for _, root := range destructiveSystemRoots {
					rootCleaned := strings.TrimRight(root, "/*")
					if rootCleaned == "" {
						rootCleaned = "/"
					}

					if cleaned == root || cleaned == rootCleaned || strings.HasPrefix(cleaned, rootCleaned+"/") || noPreserveRoot {
						return ShellRisk{
							Level:   RiskLevelCritical,
							Reason:  "Destructive recursive removal targeting root or system directory",
							Pattern: sub,
						}
					}
				}
			}
		}
	}

	return ShellRisk{Level: RiskLevelNone}
}
