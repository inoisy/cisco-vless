package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

var stdin = bufio.NewReader(os.Stdin)

func isTTY() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

func ask(q, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", q, def)
	} else {
		fmt.Printf("%s: ", q)
	}
	line, _ := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func askSecret(q string) string {
	fmt.Printf("%s: ", q)
	if !isTTY() {
		line, _ := stdin.ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}
	b, _ := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return string(b)
}

func yesNo(q string) bool {
	a := strings.ToLower(ask(q+" [y/N]", ""))
	return strings.HasPrefix(a, "y") || strings.HasPrefix(a, "д")
}

func askOTP(otp string) string {
	if otp != "" || !isTTY() {
		return otp
	}
	return ask("OTP (Enter, если не нужен)", "")
}
