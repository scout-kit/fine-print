package db

import "testing"

// A semicolon inside a `--` comment used to be treated as a statement
// boundary, chopping the following SQL in half.
func TestSplitStatements_IgnoresSemicolonsInComments(t *testing.T) {
	sql := `-- taken_at is the camera reading; NULL means absent
ALTER TABLE photos ADD COLUMN taken_at DATETIME;
ALTER TABLE photos ADD COLUMN camera_make TEXT;
`
	got := nonEmpty(splitStatements(sql))
	want := []string{
		"ALTER TABLE photos ADD COLUMN taken_at DATETIME",
		"ALTER TABLE photos ADD COLUMN camera_make TEXT",
	}
	assertStatements(t, got, want)
}

// A `--` inside a quoted literal is data, not a comment.
func TestSplitStatements_PreservesDashesInsideLiterals(t *testing.T) {
	sql := `INSERT INTO settings (key, value) VALUES ('sep', '--');
INSERT INTO settings (key, value) VALUES ('note', 'it''s -- fine');
`
	got := nonEmpty(splitStatements(sql))
	want := []string{
		`INSERT INTO settings (key, value) VALUES ('sep', '--')`,
		`INSERT INTO settings (key, value) VALUES ('note', 'it''s -- fine')`,
	}
	assertStatements(t, got, want)
}

// Trailing and standalone comments must not become statements of their own.
func TestSplitStatements_CommentOnlyLinesVanish(t *testing.T) {
	sql := `-- leading note
ALTER TABLE photos ADD COLUMN a TEXT; -- trailing note
-- dangling note with a semicolon; really
`
	got := nonEmpty(splitStatements(sql))
	assertStatements(t, got, []string{"ALTER TABLE photos ADD COLUMN a TEXT"})
}

// nonEmpty mirrors the trimming the migration runner applies before executing.
func nonEmpty(stmts []string) []string {
	out := make([]string, 0, len(stmts))
	for _, s := range stmts {
		if trimmed := trimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && isSpace(s[start]) {
		start++
	}
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func assertStatements(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d statements %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("statement %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}
