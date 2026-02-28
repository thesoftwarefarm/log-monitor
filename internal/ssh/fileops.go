package ssh

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"log-monitor/internal/logger"

	"al.essio.dev/pkg/shellescape"
	gossh "golang.org/x/crypto/ssh"
)

// CommandOpts holds optional parameters for remote command execution.
type CommandOpts struct {
	SudoPassword string
}

// FileInfo holds metadata about a remote file.
type FileInfo struct {
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

// ListFiles returns files in the given directory, optionally filtered by glob patterns.
func ListFiles(client *gossh.Client, dir string, patterns []string, opts CommandOpts) ([]FileInfo, error) {
	cmd := fmt.Sprintf("ls -la --time-style=full-iso %s", shellescape.Quote(dir))
	output, err := runCommand(client, cmd, opts)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", dir, err)
	}

	files := parseLsOutput(output)

	if len(patterns) > 0 {
		files = filterByPatterns(files, patterns)
	}

	// Sort by filename
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	return files, nil
}

// ReadFileContent reads the last N lines of a remote file.
func ReadFileContent(client *gossh.Client, path string, lines int, opts CommandOpts) (string, error) {
	cmd := fmt.Sprintf("tail -n %d %s", lines, shellescape.Quote(path))
	output, err := runCommand(client, cmd, opts)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return output, nil
}

// StatFile returns metadata for a single remote file.
func StatFile(client *gossh.Client, path string, opts CommandOpts) (*FileInfo, error) {
	cmd := fmt.Sprintf("stat --format='%%n %%s %%Y %%F' %s", shellescape.Quote(path))
	output, err := runCommand(client, cmd, opts)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) < 4 {
		return nil, fmt.Errorf("unexpected stat output: %s", output)
	}

	size, _ := strconv.ParseInt(parts[1], 10, 64)
	epoch, _ := strconv.ParseInt(parts[2], 10, 64)

	return &FileInfo{
		Name:    filepath.Base(parts[0]),
		Size:    size,
		ModTime: time.Unix(epoch, 0),
		IsDir:   parts[3] == "directory",
	}, nil
}

// DownloadFile streams a remote file to a local path via cat over SSH.
func DownloadFile(client *gossh.Client, remotePath, localPath string, opts CommandOpts) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("creating local directory: %w", err)
	}

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer sess.Close()

	cmd := fmt.Sprintf("cat %s", shellescape.Quote(remotePath))

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local file: %w", err)
	}
	defer f.Close()

	if opts.SudoPassword != "" {
		sudoCmd := fmt.Sprintf("sudo -S %s", cmd)
		logger.Log("ssh", "DownloadFile (sudo): %s → %s", remotePath, localPath)

		var stderr bytes.Buffer
		sess.Stderr = &stderr

		stdout, err := sess.StdoutPipe()
		if err != nil {
			return fmt.Errorf("stdout pipe: %w", err)
		}

		stdin, err := sess.StdinPipe()
		if err != nil {
			return fmt.Errorf("stdin pipe: %w", err)
		}

		if err := sess.Start(sudoCmd); err != nil {
			return fmt.Errorf("starting %q: %w", sudoCmd, err)
		}

		if _, err := fmt.Fprintf(stdin, "%s\n", opts.SudoPassword); err != nil {
			return fmt.Errorf("writing sudo password: %w", err)
		}
		stdin.Close()

		if _, err := io.Copy(f, stdout); err != nil {
			return fmt.Errorf("downloading file: %w", err)
		}

		if err := sess.Wait(); err != nil {
			stderrStr := stderr.String()
			if strings.Contains(stderrStr, "Sorry, try again") || strings.Contains(stderrStr, "incorrect password") {
				return fmt.Errorf("sudo authentication failed")
			}
			return fmt.Errorf("running %q: %w: %s", cmd, err, stderrStr)
		}
		return nil
	}

	logger.Log("ssh", "DownloadFile: %s → %s", remotePath, localPath)

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := sess.Start(cmd); err != nil {
		return fmt.Errorf("starting %q: %w", cmd, err)
	}

	if _, err := io.Copy(f, stdout); err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}

	if err := sess.Wait(); err != nil {
		return fmt.Errorf("running %q: %w", cmd, err)
	}

	return nil
}

func runCommand(client *gossh.Client, cmd string, opts CommandOpts) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}
	defer sess.Close()

	if opts.SudoPassword != "" {
		sudoCmd := fmt.Sprintf("sudo -S %s", cmd)
		logger.Log("ssh", "runCommand (sudo): %s", cmd)

		var stdout, stderr bytes.Buffer
		sess.Stdout = &stdout
		sess.Stderr = &stderr

		stdin, err := sess.StdinPipe()
		if err != nil {
			return "", fmt.Errorf("stdin pipe: %w", err)
		}

		if err := sess.Start(sudoCmd); err != nil {
			return "", fmt.Errorf("starting %q: %w", sudoCmd, err)
		}

		_, err = fmt.Fprintf(stdin, "%s\n", opts.SudoPassword)
		if err != nil {
			return "", fmt.Errorf("writing sudo password: %w", err)
		}
		stdin.Close()

		err = sess.Wait()
		stderrStr := stderr.String()
		if err != nil {
			if strings.Contains(stderrStr, "Sorry, try again") || strings.Contains(stderrStr, "incorrect password") {
				return "", fmt.Errorf("sudo authentication failed")
			}
			return "", fmt.Errorf("running %q: %w: %s", cmd, err, stderrStr)
		}
		return stdout.String(), nil
	}

	out, err := sess.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("running %q: %w: %s", cmd, err, string(out))
	}
	return string(out), nil
}

// parseLsOutput parses `ls -la --time-style=full-iso` output into FileInfo entries.
// Format: permissions links owner group size date time timezone name
func parseLsOutput(output string) []FileInfo {
	var files []FileInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		name := strings.Join(fields[8:], " ")
		if name == "." || name == ".." {
			continue
		}

		size, _ := strconv.ParseInt(fields[4], 10, 64)

		// Parse date and time: fields[5] = "2024-01-15", fields[6] = "10:30:00.000000000"
		dateStr := fields[5] + " " + fields[6]
		// Truncate nanosecond portion for parsing
		if idx := strings.Index(dateStr, "."); idx > 0 {
			dateStr = dateStr[:idx]
		}
		modTime, _ := time.Parse("2006-01-02 15:04:05", dateStr)

		isDir := fields[0][0] == 'd'

		files = append(files, FileInfo{
			Name:    name,
			Size:    size,
			ModTime: modTime,
			IsDir:   isDir,
		})
	}
	return files
}

func filterByPatterns(files []FileInfo, patterns []string) []FileInfo {
	var filtered []FileInfo
	for _, f := range files {
		for _, p := range patterns {
			matched, err := filepath.Match(p, f.Name)
			if err == nil && matched {
				filtered = append(filtered, f)
				break
			}
		}
	}
	return filtered
}

// FormatSize returns a human-readable file size.
func FormatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1fG", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1fM", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1fK", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
