package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
)

func TestCompactRatioHelpAndDocsUseConfigBounds(t *testing.T) {
	want := fmt.Sprintf("percentage above %g and below %g", config.CompactRatioMin*100, config.CompactRatioMax*100)
	for _, usage := range []func(){configUsage, configCompactRatioUsage} {
		if out := captureStdout(t, usage); !strings.Contains(out, want) {
			t.Fatalf("usage = %q, want %q", out, want)
		}
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "CLI.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), want) {
		t.Fatalf("CLI docs must state %q", want)
	}
}

func TestParseCLICompactRatioMatchesConfig(t *testing.T) {
	for _, value := range []string{"0", "0.5", "1", "100", "99.9", "0.05", "50", "abc", "", "70%", "-1", "NaN", "+Inf", "-Inf", " 50 "} {
		t.Run(value, func(t *testing.T) {
			_, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64)
			cfg := config.Default()
			typedRatio, _ := strconv.ParseFloat(strings.TrimSpace(value)+"e-2", 64)
			valid := parseErr == nil && cfg.SetCompactRatio(typedRatio) == nil
			ratio, err := parseCLICompactRatio(value)
			if (err == nil) != valid {
				t.Fatalf("parse error = %v, config valid = %v", err, valid)
			}
			if valid && ratio != cfg.Agent.CompactRatio {
				t.Fatalf("ratio = %v, want %v", ratio, cfg.Agent.CompactRatio)
			}
			if !valid {
				want := fmt.Sprintf("percentage above %g and below %g", config.CompactRatioMin*100, config.CompactRatioMax*100)
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v, want %q", err, want)
				}
			}
		})
	}
}

func TestCompactRatioMatchesConfigAndPreservesNeighbours(t *testing.T) {
	for _, local := range []bool{false, true} {
		for _, value := range []string{"0", "0.5", "1", "100", "99.9", "0.05", "50", "abc", "", "70%", "-1", "NaN", "+Inf", "64", "86"} {
			t.Run(strconv.FormatBool(local)+"/"+value, func(t *testing.T) {
				isolateCLIConfigHome(t)
				path := config.UserConfigPath()
				args := []string{"config", "compact-ratio"}
				source := "user:"
				if local {
					path = "reasonix.toml"
					args = append(args, "--local")
					source = "project:"
				}
				seed := config.Default()
				seed.Agent.Temperature = 0.42
				if err := seed.SaveTo(path); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				percent, parseErr := strconv.ParseFloat(value, 64)
				want := 0
				if parseErr != nil || seed.SetCompactRatio(percent/100) != nil {
					want = 2
				}
				captureStdout(t, func() {
					captureStderr(t, func() {
						if got := Run(append(args, "--", value), "test-version"); got != want {
							t.Errorf("exit = %d, config expects %d", got, want)
						}
					})
				})
				if want == 2 {
					after, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if string(after) != string(before) {
						t.Fatal("rejection changed config")
					}
					return
				}
				cfg := config.LoadForEdit(path)
				if typed, _ := strconv.ParseFloat(value+"e-2", 64); cfg.Agent.CompactRatio != typed || cfg.Agent.Temperature != 0.42 {
					t.Fatalf("saved ratio/temperature = %v/%v", cfg.Agent.CompactRatio, cfg.Agent.Temperature)
				}
				out := captureStdout(t, func() {
					if rc := Run([]string{"config", "compact-ratio"}, "test-version"); rc != 0 {
						t.Errorf("query exit = %d", rc)
					}
				})
				if !strings.Contains(out, formatCompactRatioPercent(percent/100)) || !strings.Contains(out, source) {
					t.Fatalf("query = %q", out)
				}
			})
		}
	}
}

func TestParseCLICompactRatioRefusesHugeExponentsWithoutBuildingThem(t *testing.T) {
	for _, value := range []string{"1e999999999", "1e-999999999", "9e99999999999"} {
		if _, err := parseCLICompactRatio(value); err == nil {
			t.Fatalf("%q was accepted", value)
		}
	}
}

func TestParseCLICompactRatioKeepsTheDecimalTyped(t *testing.T) {
	for value, want := range map[string]string{
		"12.3": "0.123", "33.3": "0.333", "66.6": "0.666", "87.65": "0.8765", "99.9": "0.999", "0.1": "0.001", "50": "0.5", " 7.25 ": "0.0725", "1e1": "0.1",
	} {
		ratio, err := parseCLICompactRatio(value)
		if err != nil {
			t.Fatalf("%q: %v", value, err)
		}
		if got := strconv.FormatFloat(ratio, 'f', -1, 64); got != want {
			t.Fatalf("%q -> %s, want %s", value, got, want)
		}
	}
}
