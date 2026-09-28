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
	regex   *regexp.Regexp
	level   RiskLevel
	reason  string
}

var defaultDangerPatterns = []dangerPattern{
	// Critical: Root and filesystem destruction
	{
		regex:  regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f[a-z]*\s+/(?:\*|\s|$)`),
		level:  RiskLevelCritical,
		reason: "Destructive recursive removal targeting root filesystem (/)",
	},
	{
		regex:  regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f[a-z]*\s+(?:~|\$HOME)(?:\*|\s|/|$)`),
		level:  RiskLevelCritical,
		reason: "Destructive recursive removal targeting operator home directory",
	},
	{
		regex:  regexp.MustCompile(`(?i)\b(?:mkfs|fdisk|parted|sfdisk|gdisk)\b`),
		level:  RiskLevelCritical,
		reason: "Direct block device formatting or partition table manipulation",
	},
	{
		regex:  regexp.MustCompile(`(?i)\bdd\s+.*of=/dev/(?:sd[a-z]|nvme[0-9]|vd[a-z]|hd[a-z])`),
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
		regex:  regexp.MustCompile(`(?i)\b(?:sudo|doas|pkexec|su\s+-?)\b`),
		level:  RiskLevelHigh,
		reason: "Privilege escalation attempt requiring superuser credentials",
	},

	// High: Untrusted remote code execution or obfuscated decode pipes
	{
		regex:  regexp.MustCompile(`(?i)\b(?:curl|wget|fetch|lynx)\s+[^|]+\|\s*(?:bash|sh|zsh|dash|python|perl|ruby)\b`),
		level:  RiskLevelHigh,
		reason: "Piping unverified remote web content directly into shell interpreter",
	},
	{
		regex:  regexp.MustCompile(`(?i)\bbase64\s+(?:-d|--decode)\s*\|\s*(?:bash|sh|zsh|dash|python|perl)\b`),
		level:  RiskLevelHigh,
		reason: "Piping decoded base64 payload directly into shell interpreter",
	},
	{
		regex:  regexp.MustCompile(`(?i)\b(?:export\s+)?LD_PRELOAD=`),
		level:  RiskLevelHigh,
		reason: "Dynamic library preload injection attack (LD_PRELOAD)",
	},

	// High: Sensitive credential exfiltration or traversal
	{
		regex:  regexp.MustCompile(`(?i)\b(?:cat|head|tail|less|more|cp|mv)\s+.*(?:~|\$HOME|\/home\/[^\/]+)\/\.(?:ssh|aws|gnupg|docker\/config\.json|config\/gh)`),
		level:  RiskLevelHigh,
		reason: "Attempt to access host authentication credentials or private keys",
	},
	{
		regex:  regexp.MustCompile(`(?i)\b(?:cat|head|tail|less|more|cp|mv|grep|awk|sed)\s+.*\/etc\/(?:shadow|sudoers|master\.passwd|passwd)\b`),
		level:  RiskLevelHigh,
		reason: "Attempt to access sensitive host security and password databases",
	},
	{
		regex:  regexp.MustCompile(`(?i)\bgetent\s+(?:passwd|shadow)\b`),
		level:  RiskLevelHigh,
		reason: "Attempt to dump host user and authentication databases",
	},

	// Medium: Modifying global system directories or service configs
	{
		regex:  regexp.MustCompile(`(?i)\b(?:chmod|chown|chgrp)\s+.*(?:\s|/)\/(?:etc|bin|sbin|usr|lib|boot|sys|proc)\b`),
		level:  RiskLevelMedium,
		reason: "Permission tampering on global system directories",
	},
	{
		regex:  regexp.MustCompile(`(?i)(?:>|>>)\s*\/etc\/(?:cron|systemd|init\.d|pam\.d)`),
		level:  RiskLevelHigh,
		reason: "Attempt to write directly into system service or cron scheduler configurations",
	},
}

// InspectShellRisk analyzes a shell command for dangerous operations and host system risks.
func InspectShellRisk(workDir string, command string) ShellRisk {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ShellRisk{Level: RiskLevelNone}
	}

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
