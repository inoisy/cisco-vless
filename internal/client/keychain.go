package client

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Сервис в Keychain тот же, что у bash-версии: пароль переживает обновление.
const keychainService = "cvpn"

func keychainGet(user string) (string, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password",
		"-s", keychainService, "-a", user, "-w").Output()
	if err != nil {
		return "", errors.New("cisco: нет пароля в Keychain — cvpn password или окно настроек")
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// keychainSet — через security -i: пароль идёт в stdin, а не в аргументы,
// которые видно в ps.
func keychainSet(user, pw string) error {
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n",
		quote(keychainService), quote(user), quote(pw)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Keychain: %v: %s", err, out)
	}
	return nil
}

// quote — кавычки для командной строки security -i.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
