// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/boggycreek/lokol/liblokol/agent"
)

// RunInteractiveREPL starts an interactive command-line session directly driving SessionCore.
func RunInteractiveREPL(client *agent.Client, workDir string, initialMode agent.Mode) error {
	session := agent.NewSessionWithMode(client, workDir, initialMode)

	fmt.Println("⚡ lokol Core Session Interactive Driver")
	fmt.Printf("Working Directory: %s\n", session.GetWorkDir())
	fmt.Printf("Initial Mode:      %s\n", session.GetMode())
	fmt.Println("Type your prompt, or use /help for session commands. (Ctrl+C to abort turn / Ctrl+D to exit)")
	fmt.Println("--------------------------------------------------------------------------------")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		promptPrefix := fmt.Sprintf("[%s]> ", session.GetMode())
		fmt.Print(promptPrefix)

		if !scanner.Scan() {
			fmt.Println("\nExiting session.")
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(line, "/") {
			parts := strings.Fields(line)
			cmd := strings.ToLower(parts[0])

			switch cmd {
			case "/help":
				printREPLHelp()
				continue
			case "/mode":
				if len(parts) < 2 {
					fmt.Println("Usage: /mode <general|coding|moe>")
					continue
				}
				newMode, err := agent.ParseMode(parts[1])
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					continue
				}
				session.SetMode(newMode)
				fmt.Printf("✓ Active mode switched to: %s\n", session.GetMode())
				continue
			case "/slot":
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				status, err := session.GetSlotStatus(ctx)
				cancel()
				if err != nil {
					fmt.Printf("Error querying slot status: %v\n", err)
				} else {
					fmt.Printf("Slot Metrics: ID=%d | Context=%d/%d | Processing=%v | Decoded=%d\n",
						status.ID, status.NPromptTokens, status.NCtx, status.IsProcessing, status.NDecoded)
				}
				continue
			case "/history":
				fmt.Printf("--- Conversation History (%d messages) ---\n", len(session.History))
				for i, m := range session.History {
					contentPreview := m.Content
					if len(contentPreview) > 200 {
						contentPreview = contentPreview[:200] + "...[truncated]"
					}
					fmt.Printf("[%d] %s: %s\n", i, strings.ToUpper(m.Role), contentPreview)
				}
				fmt.Println("-----------------------------------------")
				continue
			case "/reset":
				session.Reset()
				fmt.Println("✓ Session conversation history reset.")
				continue
			case "/clear":
				fmt.Print("\033[H\033[2J")
				continue
			case "/quit", "/exit":
				fmt.Println("Exiting session.")
				return nil
			default:
				fmt.Printf("Unknown command %q. Type /help for available commands.\n", cmd)
				continue
			}
		}

		// Feed user prompt into SessionCore
		session.AppendUserMessage(line)

		// Set up signal context so Ctrl+C cancels the active turn without killing the REPL
		sigCtx, sigCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		startTurn := time.Now()

		executeTurnLoop(sigCtx, session)
		sigCancel()

		duration := time.Since(startTurn)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		slot, _ := session.GetSlotStatus(ctx)
		cancel()

		slotInfo := ""
		if slot != nil {
			slotInfo = fmt.Sprintf(" | Context: %d/%d tokens", slot.NPromptTokens, slot.NCtx)
		}
		fmt.Printf("\n[Turn completed in %.2fs%s]\n\n", duration.Seconds(), slotInfo)
	}

	return scanner.Err()
}

func executeTurnLoop(ctx context.Context, session *agent.Session) {
	maxSubturns := 15
	for subturn := 0; subturn < maxSubturns; subturn++ {
		select {
		case <-ctx.Done():
			fmt.Println("\n[Turn cancelled by user]")
			_ = session.Abort(context.Background())
			return
		default:
		}

		tokenChan := make(chan string, 128)
		errChan := make(chan error, 1)

		go func() {
			_, err := session.StreamTurn(ctx, tokenChan)
			close(tokenChan)
			errChan <- err
		}()

		var reply strings.Builder
		for tok := range tokenChan {
			reply.WriteString(tok)
			fmt.Print(tok)
		}

		if err := <-errChan; err != nil {
			if ctx.Err() != nil {
				fmt.Println("\n[Turn cancelled]")
				_ = session.Abort(context.Background())
				return
			}
			fmt.Printf("\n[Stream Error: %v]\n", err)
			return
		}

		fullReply := reply.String()
		session.AppendAssistantMessage(fullReply)

		act := agent.ParseAction(fullReply)

		if act == nil {
			// Finished turn
			break
		}

		if act.Name == "task_finish" || act.Name == "finish" {
			fmt.Printf("\n⚡ [Task Finish]: %s\n", act.Command)
			break
		}

		fmt.Printf("\n⚡ [Action: %s]\n", act.Name)
		out, execErr := session.ExecuteAction(ctx, act)
		if execErr != nil {
			fmt.Printf("   Error: %v\n", execErr)
		}
		if out != "" {
			fmt.Printf("   Output:\n%s\n", indent(out, "   "))
		}

		session.AppendActionResult(out, execErr)
	}
}

func printREPLHelp() {
	fmt.Println("Available Session Commands:")
	fmt.Println("  /mode <mode>   - Change active persona mode (general, coding, moe)")
	fmt.Println("  /slot          - Query inference engine slot metrics & context usage")
	fmt.Println("  /history       - Display message history in current session")
	fmt.Println("  /reset         - Clear conversation history back to initial system prompt")
	fmt.Println("  /clear         - Clear terminal screen")
	fmt.Println("  /quit, /exit   - Terminate session and exit")
	fmt.Println("  /help          - Display this help message")
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
