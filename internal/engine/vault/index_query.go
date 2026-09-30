package vault

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const trigramLength = 3

// searchSQL builds the vault lane's query. Matching is a per-token substring
// test across the arrow's identity and text fields; the FTS subquery, present
// only when some token is long enough for a trigram, contributes a rank and
// never a filter.
func searchSQL(
	tokens []string,
	os domain.OS,
	now time.Time,
) (string, []any) {
	var args []any
	var sql string

	order := ` ORDER BY a.stars DESC, a.namespace, a.ref`
	join := ""
	if match := ftsRankQuery(tokens); match != "" {
		// MATERIALIZED keeps SQLite from re-running the FTS scan and its bm25
		// scoring for every outer row.
		sql = `WITH f AS MATERIALIZED (
			SELECT namespace, ref, bm25(vault_arrows_fts, 0.0, 0.0, 10.0, 2.0, 5.0) AS score
			FROM vault_arrows_fts
			WHERE vault_arrows_fts MATCH ?
		) `
		join = ` LEFT JOIN f ON f.namespace = a.namespace AND f.ref = a.ref`
		args = append(args, match)
		order = ` ORDER BY (f.score IS NULL), f.score, a.stars DESC, a.namespace, a.ref`
	}

	sql += `SELECT a.namespace, a.ref, a.name, a.description, a.license, a.url,
		       a.icon, a.banner, a.stars, a.source, a.branch, a.seen_at,
		       a.generator, a.confidence
		FROM vault_arrows a` + join

	sql += ` WHERE a.row_expire_at > ?`
	args = append(args, now.Unix())

	for _, token := range tokens {
		sql += ` AND ` + termMatchClause
		pattern := likePattern(token)
		args = append(args, pattern, pattern, pattern, pattern)
	}

	if os != "" {
		sql += ` AND EXISTS (
			SELECT 1 FROM vault_arrow_os o
			WHERE o.namespace = a.namespace AND o.ref = a.ref AND o.os = ?
		)`
		args = append(args, string(os))
	}

	return sql + order, args
}

const termMatchClause = `(
	a.namespace LIKE ? ESCAPE '\' OR
	a.name LIKE ? ESCAPE '\' OR
	a.description LIKE ? ESCAPE '\' OR
	EXISTS (
		SELECT 1 FROM vault_arrow_tags g
		WHERE g.namespace = a.namespace AND g.ref = a.ref AND g.tag LIKE ? ESCAPE '\'
	)
)`

// ftsRankQuery joins every token long enough for the trigram tokenizer as an
// FTS5 OR of quoted phrases, so punctuation that is query syntax to FTS5
// (-, ", *, OR) is matched literally. It is empty when no token qualifies.
func ftsRankQuery(
	tokens []string,
) string {
	phrases := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if utf8.RuneCountInString(token) < trigramLength {
			continue
		}
		phrases = append(phrases, `"`+strings.ReplaceAll(token, `"`, `""`)+`"`)
	}
	return strings.Join(phrases, " OR ")
}

func likePattern(
	token string,
) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(token)
	return "%" + escaped + "%"
}

// firstNamespaces keeps every row of the first limit distinct bare namespaces,
// in rank order, so the limit bounds arrows rather than refs.
func firstNamespaces(
	rows []arrowIndexRow,
	limit int,
) []arrowIndexRow {
	kept := make([]arrowIndexRow, 0, len(rows))
	seen := make(map[string]struct{}, limit)
	for _, row := range rows {
		if _, ok := seen[row.Namespace]; !ok && len(seen) == limit {
			continue
		}
		seen[row.Namespace] = struct{}{}
		kept = append(kept, row)
	}
	return kept
}
