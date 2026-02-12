package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
)

// openStore creates a Store for the current directory.
func openStore() (*store.Store, error) {
	if projectDir := strings.TrimSpace(os.Getenv("ADAF_PROJECT_DIR")); projectDir != "" {
		return store.New(projectDir)
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	return store.New(dir)
}

// openStoreRequired creates a Store and checks that the project exists.
func openStoreRequired() (*store.Store, error) {
	s, err := openStore()
	if err != nil {
		return nil, err
	}
	if !s.Exists() {
		return nil, fmt.Errorf("no adaf project found (run 'adaf init' first)")
	}
	return s, nil
}

// printHeader prints a formatted section header.
func printHeader(title string) {
	fmt.Printf("\n%s%s%s\n", styleBoldCyan, title, colorReset)
	fmt.Println(colorDim + strings.Repeat("-", len(title)+2) + colorReset)
}

// printField prints a labeled field.
func printField(label, value string) {
	fmt.Printf("  %s%-16s%s %s\n", colorBold, label+":", colorReset, value)
}

// printFieldColored prints a labeled field with colored value.
func printFieldColored(label, value, color string) {
	fmt.Printf("  %s%-16s%s %s%s%s\n", colorBold, label+":", colorReset, color, value, colorReset)
}

// statusColor returns an ANSI color code for a given status string.
func statusColor(status string) string {
	switch strings.ToLower(status) {
	case "complete", "resolved", "done":
		return colorGreen
	case "in_progress", "in-progress":
		return colorYellow
	case "blocked", "critical":
		return colorRed
	case "open", "not_started", "not-started":
		return colorBlue
	case "wontfix":
		return colorDim
	case "high":
		return colorRed
	case "medium":
		return colorYellow
	case "low":
		return colorGreen
	default:
		return colorWhite
	}
}

// statusBadge returns a colored status badge.
func statusBadge(status string) string {
	return fmt.Sprintf("%s[%s]%s", statusColor(status), status, colorReset)
}

// priorityBadge returns a colored priority badge.
func priorityBadge(priority string) string {
	return fmt.Sprintf("%s(%s)%s", statusColor(priority), priority, colorReset)
}

// printTable prints a simple table with headers and rows.
func printTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println(colorDim + "  (none)" + colorReset)
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				// Strip ANSI codes for width calculation
				stripped := stripAnsi(cell)
				if len(stripped) > widths[i] {
					widths[i] = len(stripped)
				}
			}
		}
	}

	// Print header
	headerLine := "  "
	for i, h := range headers {
		headerLine += fmt.Sprintf("%s%-*s%s", colorBold, widths[i]+2, h, colorReset)
	}
	fmt.Println(headerLine)

	// Print separator
	sepLine := "  "
	for _, w := range widths {
		sepLine += colorDim + strings.Repeat("-", w+2) + colorReset
	}
	fmt.Println(sepLine)

	// Print rows
	for _, row := range rows {
		rowLine := "  "
		for i, cell := range row {
			if i < len(widths) {
				// For proper alignment with ANSI codes, we need to account for invisible chars
				stripped := stripAnsi(cell)
				padding := widths[i] - len(stripped)
				if padding < 0 {
					padding = 0
				}
				rowLine += cell + strings.Repeat(" ", padding+2)
			}
		}
		fmt.Println(rowLine)
	}
}

// stripAnsi removes ANSI escape codes from a string (for width calculation).
func stripAnsi(s string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// truncate truncates a string to a given max length, adding "..." if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// firstLine returns the first line of a multi-line string.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
