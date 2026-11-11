/*
  Copyright © 2026 Alexey Shulutkov <github@shulutkov.ru>

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  	http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package awg

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ks-tool/vpn-agent/models"
)

const firewallExecutable = "/usr/local/bin/vpn-provider-awg"

type firewallRequest struct {
	kind       string
	action     string
	iface      string
	listenPort uint16
	mark       string
}

type netfilterRule struct {
	id    string
	table string
	chain string
	args  []string
}

type firewallSystem interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) (string, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
}

type execFirewallSystem struct{}

func (execFirewallSystem) Run(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (execFirewallSystem) Output(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (execFirewallSystem) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }
func (execFirewallSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func firewallHook(kind, action string, listenPort uint16, mark string) string {
	command := fmt.Sprintf("%s firewall %s %s --interface %%i", firewallExecutable, kind, action)
	if listenPort != 0 {
		command += " --listen " + strconv.Itoa(int(listenPort))
	}
	if mark != "" {
		command += " --mark " + mark
	}
	return command
}

func interfaceFirewallHooks(cfg models.InterfaceConfig) (string, string) {
	return firewallHook("interface", "add", cfg.ListenPort, ""), firewallHook("interface", "remove", cfg.ListenPort, "")
}

func replaceInterfaceFirewallHooks(cfg *models.InterfaceConfig) {
	cfg.PreUp = removeInterfaceFirewallHooks(cfg.PreUp)
	cfg.PostDown = removeInterfaceFirewallHooks(cfg.PostDown)
	up, down := interfaceFirewallHooks(*cfg)
	cfg.PreUp = append([]string{up}, cfg.PreUp...)
	cfg.PostDown = append([]string{down}, cfg.PostDown...)
}

func removeInterfaceFirewallHooks(commands []string) []string {
	remaining := make([]string, 0, len(commands))
	for _, command := range commands {
		if strings.HasPrefix(command, firewallExecutable+" firewall interface ") {
			continue
		}
		remaining = append(remaining, command)
	}
	return remaining
}

func runFirewallCommand(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: vpn-provider-awg firewall <interface|transit|exit> <add|remove> --interface NAME [--listen PORT] [--mark MARK]")
	}
	request := firewallRequest{kind: args[0], action: args[1]}
	flags := flag.NewFlagSet("firewall", flag.ContinueOnError)
	iface := flags.String("interface", "", "AWG interface")
	listen := flags.Uint("listen", 0, "AWG UDP listen port")
	mark := flags.String("mark", "", "packet mark")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	request.iface, request.listenPort, request.mark = *iface, uint16(*listen), *mark
	if request.kind != "interface" && request.kind != "transit" && request.kind != "exit" {
		return fmt.Errorf("unsupported firewall rule group %q", request.kind)
	}
	if request.action != "add" && request.action != "remove" {
		return fmt.Errorf("unsupported firewall action %q", request.action)
	}
	if request.iface == "" || request.kind == "exit" && request.mark == "" {
		return errors.New("interface and exit mark are required for their respective rule groups")
	}
	return applyFirewallRequest(ctx, request, execFirewallSystem{})
}

func applyFirewallRequest(_ context.Context, request firewallRequest, system firewallSystem) error {
	backend := "iptables"
	_, serviceErr := system.Output("systemctl", "is-active", "--quiet", "firewalld")
	_, processErr := system.Output("pgrep", "-x", "firewalld")
	if serviceErr == nil || processErr == nil {
		backend = "firewalld"
	} else if output, err := system.Output("ufw", "status"); err == nil && strings.Contains(output, "Status: active") {
		backend = "ufw"
	}

	switch backend {
	case "firewalld":
		if request.kind == "interface" {
			return applyFirewalldInterface(system, request)
		}
		for _, rule := range firewallNetfilterRules(request) {
			if err := applyFirewalldRule(system, rule, request.action == "add"); err != nil {
				return err
			}
		}
		return nil
	case "ufw":
		if request.kind == "interface" {
			return applyUFWFilterRules(system, request)
		}
		for _, rule := range firewallNetfilterRules(request) {
			if err := applyUFWNetfilterRule(system, rule, request.action == "add"); err != nil {
				return err
			}
		}
		return nil
	default:
		for _, rule := range firewallNetfilterRules(request) {
			if err := applyIptablesRule(system, rule, request.action == "add"); err != nil {
				return err
			}
		}
		return nil
	}
}

func firewallNetfilterRules(request firewallRequest) []netfilterRule {
	switch request.kind {
	case "interface":
		rules := []netfilterRule{
			{id: "input-" + request.iface, table: "filter", chain: "INPUT", args: []string{"-i", request.iface, "-j", "ACCEPT"}},
			{id: "forward-" + request.iface, table: "filter", chain: "FORWARD", args: []string{"-i", request.iface, "-j", "ACCEPT"}},
		}
		if request.listenPort != 0 {
			rules = append(rules, netfilterRule{id: "listen-" + request.iface, table: "filter", chain: "INPUT", args: []string{"-p", "udp", "--dport", strconv.Itoa(int(request.listenPort)), "-j", "ACCEPT"}})
		}
		return rules
	case "transit":
		return []netfilterRule{{id: "transit-" + request.iface, table: "nat", chain: "POSTROUTING", args: []string{"-o", request.iface, "-j", "MASQUERADE"}}}
	case "exit":
		return []netfilterRule{
			{id: "exit-" + request.iface, table: "mangle", chain: "PREROUTING", args: []string{"-i", request.iface, "-j", "MARK", "--set-mark", request.mark}},
			{id: "exit-" + request.iface, table: "nat", chain: "POSTROUTING", args: []string{"!", "-o", request.iface, "-m", "mark", "--mark", request.mark, "-j", "MASQUERADE"}},
		}
	default:
		return nil
	}
}

func applyFirewalldInterface(system firewallSystem, request firewallRequest) error {
	add := request.action == "add"
	if add {
		if err := ensureFirewalldForwardPolicy(system); err != nil {
			return err
		}
	}
	for _, permanent := range []bool{false, true} {
		prefix := []string{}
		if permanent {
			prefix = append(prefix, "--permanent")
		}
		if err := applyFirewalldOption(system, prefix, []string{"--zone=trusted", "--query-interface=" + request.iface}, []string{"--zone=trusted", "--change-interface=" + request.iface}, []string{"--zone=trusted", "--remove-interface=" + request.iface}, add); err != nil {
			return err
		}
		if add {
			if err := applyFirewalldOption(system, prefix, []string{"--zone=trusted", "--query-forward"}, []string{"--zone=trusted", "--add-forward"}, nil, true); err != nil {
				return err
			}
		}
		if request.listenPort != 0 {
			port := strconv.Itoa(int(request.listenPort)) + "/udp"
			if err := applyFirewalldOption(system, prefix, []string{"--query-port=" + port}, []string{"--add-port=" + port}, []string{"--remove-port=" + port}, add); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyFirewalldOption(system firewallSystem, prefix, query, addCommand, removeCommand []string, add bool) error {
	_, queryErr := system.Output("firewall-cmd", append(append([]string{}, prefix...), query...)...)
	if add == (queryErr == nil) {
		return nil
	}
	command := addCommand
	if !add {
		if removeCommand == nil {
			return nil
		}
		command = removeCommand
	}
	return system.Run("firewall-cmd", append(append([]string{}, prefix...), command...)...)
}

func ensureFirewalldForwardPolicy(system firewallSystem) error {
	const name = "vpn-admin-forward"
	output, err := system.Output("firewall-cmd", "--get-policies")
	if err == nil && strings.Contains(output, name) {
		return nil
	}
	for _, args := range [][]string{
		{"--permanent", "--new-policy=" + name},
		{"--permanent", "--policy=" + name, "--add-ingress-zone=trusted"},
		{"--permanent", "--policy=" + name, "--add-egress-zone=ANY"},
		{"--permanent", "--policy=" + name, "--set-target=ACCEPT"},
	} {
		if err := system.Run("firewall-cmd", args...); err != nil {
			return err
		}
	}
	return system.Run("firewall-cmd", "--reload")
}

func applyFirewalldRule(system firewallSystem, rule netfilterRule, add bool) error {
	for _, permanent := range []bool{false, true} {
		prefix := []string{}
		if permanent {
			prefix = append(prefix, "--permanent")
		}
		query := append(append([]string{}, prefix...), "--direct", "--query-rule", "ipv4", rule.table, rule.chain, "0")
		query = append(query, rule.args...)
		_, queryErr := system.Output("firewall-cmd", query...)
		if add == (queryErr == nil) {
			continue
		}
		action := "--add-rule"
		if !add {
			action = "--remove-rule"
		}
		command := append(append([]string{}, prefix...), "--direct", action, "ipv4", rule.table, rule.chain, "0")
		command = append(command, rule.args...)
		if err := system.Run("firewall-cmd", command...); err != nil {
			return err
		}
	}
	return nil
}

func applyUFWFilterRules(system firewallSystem, request firewallRequest) error {
	commands := [][]string{
		{"allow", "in", "on", request.iface},
		{"route", "allow", "in", "on", request.iface},
	}
	if request.listenPort != 0 {
		commands = append(commands, []string{"allow", strconv.Itoa(int(request.listenPort)) + "/udp"})
	}
	for _, command := range commands {
		if request.action == "remove" {
			command = append([]string{"delete"}, command...)
		}
		if err := system.Run("ufw", command...); err != nil {
			return err
		}
	}
	return nil
}

func applyUFWNetfilterRule(system firewallSystem, rule netfilterRule, add bool) error {
	data, err := system.ReadFile("/etc/ufw/before.rules")
	if err != nil {
		return err
	}
	updated, owned, changed := setUFWNetfilterRule(string(data), rule, add)
	if !add && !owned {
		return nil
	}
	if changed {
		if err := system.WriteFile("/etc/ufw/before.rules", []byte(updated), 0o640); err != nil {
			return err
		}
	}
	return applyIptablesRule(system, rule, add)
}

func applyIptablesRule(system firewallSystem, rule netfilterRule, add bool) error {
	check := []string{"-t", rule.table, "-C", rule.chain}
	check = append(check, rule.args...)
	_, queryErr := system.Output("iptables", check...)
	if add == (queryErr == nil) {
		return nil
	}
	action := "-A"
	if !add {
		action = "-D"
	}
	command := []string{"-t", rule.table, action, rule.chain}
	command = append(command, rule.args...)
	return system.Run("iptables", command...)
}

func setUFWNetfilterRule(content string, rule netfilterRule, add bool) (string, bool, bool) {
	marker := "# vpn-admin:" + rule.id
	ruleLine := "-A " + rule.chain + " " + strings.Join(rule.args, " ")
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if line == "*"+rule.table {
			start = i
			break
		}
	}
	if start == -1 {
		if !add {
			return content, false, false
		}
		lines = append(lines, "*"+rule.table, marker, ruleLine, "COMMIT", "")
		return strings.Join(lines, "\n"), true, true
	}
	end := -1
	for i := start + 1; i < len(lines); i++ {
		if lines[i] == "COMMIT" {
			end = i
			break
		}
	}
	if end == -1 {
		return content, false, false
	}
	if add {
		for i := start + 1; i < end; i++ {
			if lines[i] == marker {
				return content, true, false
			}
		}
	}
	owned := false
	filtered := make([]string, 0, len(lines)+2)
	filtered = append(filtered, lines[:start+1]...)
	for i := start + 1; i < end; i++ {
		if lines[i] == marker {
			owned = true
			if i+1 < end && lines[i+1] == ruleLine {
				i++
			}
			continue
		}
		filtered = append(filtered, lines[i])
	}
	if add && !owned {
		exists := false
		for _, line := range filtered[start+1:] {
			if line == ruleLine {
				exists = true
				break
			}
		}
		if !exists {
			filtered = append(filtered, marker, ruleLine)
			owned = true
		}
	}
	filtered = append(filtered, lines[end:]...)
	updated := strings.Join(filtered, "\n")
	return updated, owned, updated != content
}
