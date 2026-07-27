package vault

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const testIndexTTL = 720 * time.Hour

func newTestIndex(t *testing.T) *index {
	t.Helper()
	idx, err := openIndex(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = idx.close() })
	return idx
}

func testMeta() IndexMeta {
	return IndexMeta{
		Arrow: domain.ArrowMeta{
			Name:        "Chromium",
			Description: "A fast web browser",
			Tags:        []string{"browser", "web"},
			Media:       domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"},
		},
		OS:     []domain.OS{domain.OSLinuxAMD64},
		Stars:  42,
		Source: "github.com",
		Branch: "main",
	}
}

func TestOpenIndex_CreatesSchema(t *testing.T) {
	idx, err := openIndex(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	require.NotNil(t, idx)
	t.Cleanup(func() { _ = idx.close() })

	var n int64
	require.NoError(t, idx.db.Raw(
		`SELECT count(*) FROM sqlite_master WHERE name = 'vault_arrows_fts'`,
	).Scan(&n).Error)
	require.Equal(t, int64(1), n, "FTS5 virtual table must exist")
}

func TestOpenIndex_IsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	first, err := openIndex(path)
	require.NoError(t, err)
	require.NoError(t, first.close())

	second, err := openIndex(path)
	require.NoError(t, err)
	require.NoError(t, second.close())
}

// TestOpenIndex_ColumnsAreStable pins every column name and its position.
// AutoMigrate only ever adds columns, so a renamed one becomes a second, empty
// column and orphans the cached rows without raising an error. Refactors that
// move fields between row structs and embedded domain types have to leave this
// list untouched.
func TestOpenIndex_ColumnsAreStable(t *testing.T) {
	testCases := []struct {
		name    string
		table   string
		columns []string
	}{
		{
			name:  "vault_arrows",
			table: "vault_arrows",
			columns: []string{
				"namespace", "ref", "name", "description", "icon", "banner",
				"stars", "source", "filename", "branch", "seen_at", "row_expire_at",
			},
		},
		{
			name:    "vault_arrow_tags",
			table:   "vault_arrow_tags",
			columns: []string{"namespace", "ref", "tag"},
		},
		{
			name:    "vault_arrow_os",
			table:   "vault_arrow_os",
			columns: []string{"namespace", "ref", "os"},
		},
	}

	idx := newTestIndex(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var columns []string
			require.NoError(t, idx.db.Raw(
				`SELECT name FROM pragma_table_info(?) ORDER BY cid`, tc.table,
			).Scan(&columns).Error)
			require.Equal(t, tc.columns, columns)
		})
	}
}

func TestIndex_Upsert_InsertsRowAndIsSearchable(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	require.NoError(t, idx.upsert(
		"github.com/user/repo@v1",
		ManifestFile{Filename: "ARROW.md"},
		testMeta(),
		now,
		testIndexTTL,
	))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "Chromium", rows[0].Meta.Arrow.Name)
	require.Equal(t, "v1", rows[0].Ref)
	require.Equal(t, "main", rows[0].Meta.Branch)
}

func TestIndex_Upsert_IsIdempotent(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	ns := domain.Namespace("github.com/user/repo@v1")

	for range 3 {
		require.NoError(t, idx.upsert(ns, ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))
	}

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
	require.NoError(t, err)
	require.Len(t, rows, 1, "repeated upserts must not duplicate rows or FTS entries")
}

func TestIndex_Upsert_TwoRefsAreSeparateRows(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	require.NoError(t, idx.upsert("github.com/user/repo@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))
	require.NoError(t, idx.upsert("github.com/user/repo@v2", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestIndex_Upsert_SlidingExpiryResetsClock(t *testing.T) {
	idx := newTestIndex(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := domain.Namespace("github.com/user/repo@v1")

	require.NoError(t, idx.upsert(ns, ManifestFile{Filename: "ARROW.md"}, testMeta(), t0, testIndexTTL))

	// Re-save at 29 days — inside the window, so the clock resets.
	t1 := t0.Add(29 * 24 * time.Hour)
	require.NoError(t, idx.upsert(ns, ManifestFile{Filename: "ARROW.md"}, testMeta(), t1, testIndexTTL))

	// 29 more days: 58 total, well past the 30d TTL, but the re-save moved it.
	t2 := t1.Add(29 * 24 * time.Hour)
	require.NoError(t, idx.evictExpired(t2))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, t2)
	require.NoError(t, err)
	require.Len(t, rows, 1, "re-save must slide row_expire_at forward")
	require.True(t, rows[0].SeenAt.Equal(t1), "seen_at must move on re-save")
}

func TestIndex_Search_TrigramMatchesSubstring(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	for _, q := range []string{"chrom", "Chromium", "rowser", "browser"} {
		rows, err := idx.search(IndexQuery{Text: q, Limit: 10}, now)
		require.NoError(t, err, q)
		require.Len(t, rows, 1, "query %q should match name, description or tag", q)
	}
}

func TestIndex_Search_NoMatchReturnsEmptyNotError(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	rows, err := idx.search(IndexQuery{Text: "zzzzz", Limit: 10}, now)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestIndex_Search_OSFilterExcludes(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	match, err := idx.search(IndexQuery{Text: "chrom", OS: domain.OSLinuxAMD64, Limit: 10}, now)
	require.NoError(t, err)
	require.Len(t, match, 1)

	none, err := idx.search(IndexQuery{Text: "chrom", OS: domain.OSWindowsAMD64, Limit: 10}, now)
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestIndex_Search_ExcludesExpiredRowsBeforeSweep(t *testing.T) {
	idx := newTestIndex(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), t0, 24*time.Hour))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, t0.Add(48*time.Hour))
	require.NoError(t, err)
	require.Empty(t, rows, "an expired row must not surface even if the sweep has not run")
}

func TestIndex_Search_RespectsLimit(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	for _, ref := range []string{"v1", "v2", "v3"} {
		require.NoError(t, idx.upsert(
			domain.Namespace("github.com/u/r@"+ref),
			ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL,
		))
	}

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 2}, now)
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestIndex_Search_EmptyTextReturnsEmpty(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	rows, err := idx.search(IndexQuery{Text: "  ", Limit: 10}, now)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestIndex_EvictExpired_RemovesRowAndFTS(t *testing.T) {
	idx := newTestIndex(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), t0, 24*time.Hour))

	require.NoError(t, idx.evictExpired(t0.Add(48*time.Hour)))

	var n int64
	require.NoError(t, idx.db.Raw(`SELECT count(*) FROM vault_arrows_fts`).Scan(&n).Error)
	require.Equal(t, int64(0), n, "FTS entry must be removed with the row")

	var tags int64
	require.NoError(t, idx.db.Raw(`SELECT count(*) FROM vault_arrow_tags`).Scan(&tags).Error)
	require.Equal(t, int64(0), tags, "child rows must be removed with the row")

	var oses int64
	require.NoError(t, idx.db.Raw(`SELECT count(*) FROM vault_arrow_os`).Scan(&oses).Error)
	require.Equal(t, int64(0), oses, "os rows must be removed with the row")
}

func TestIndex_EvictExpired_KeepsLiveRows(t *testing.T) {
	idx := newTestIndex(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), t0, testIndexTTL))

	require.NoError(t, idx.evictExpired(t0.Add(48*time.Hour)))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, t0.Add(48*time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestIndex_UpsertAfterEviction_CreatesFreshRow(t *testing.T) {
	idx := newTestIndex(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := domain.Namespace("github.com/u/r@v1")
	require.NoError(t, idx.upsert(ns, ManifestFile{Filename: "ARROW.md"}, testMeta(), t0, 24*time.Hour))

	t1 := t0.Add(48 * time.Hour)
	require.NoError(t, idx.evictExpired(t1))
	require.NoError(t, idx.upsert(ns, ManifestFile{Filename: "ARROW.md"}, testMeta(), t1, testIndexTTL))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, t1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].SeenAt.Equal(t1), "resurrected row must carry fresh fields, not stale ones")
}

func TestIndex_Forget_RemovesAllRefs(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))
	require.NoError(t, idx.upsert("github.com/u/r@v2", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	require.NoError(t, idx.forget("github.com/u/r"))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
	require.NoError(t, err)
	require.Empty(t, rows)

	var tags int64
	require.NoError(t, idx.db.Raw(`SELECT count(*) FROM vault_arrow_tags`).Scan(&tags).Error)
	require.Equal(t, int64(0), tags)

	var oses int64
	require.NoError(t, idx.db.Raw(`SELECT count(*) FROM vault_arrow_os`).Scan(&oses).Error)
	require.Equal(t, int64(0), oses)
}

func TestIndex_Forget_UnknownNamespaceIsNoOp(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.forget("github.com/nope/nope"))
}

func TestIndex_Forget_RefIsStrippedFromNamespace(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	require.NoError(t, idx.forget("github.com/u/r@v1"))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestIndex_Search_HydratesFullMetadata(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	got := rows[0]
	require.Equal(t, domain.Namespace("github.com/u/r"), got.Namespace)
	require.Equal(t, "A fast web browser", got.Meta.Arrow.Description)
	require.Equal(t, []string{"browser", "web"}, got.Meta.Arrow.Tags)
	require.Equal(t, "icon.png", got.Meta.Arrow.Media.Icon)
	require.Equal(t, "banner.png", got.Meta.Arrow.Media.Banner)
	require.Equal(t, []domain.OS{domain.OSLinuxAMD64}, got.Meta.OS)
	require.Equal(t, 42, got.Meta.Stars)
	require.Equal(t, "github.com", got.Meta.Source)
	require.True(t, got.SeenAt.Equal(now))
}

func TestIndex_Search_ZeroLimitUsesDefault(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))

	rows, err := idx.search(IndexQuery{Text: "chrom"}, now)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestIndex_Search_QuerySyntaxCharactersAreLiteral(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	meta := testMeta()
	meta.Arrow.Name = "quiver-core"
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, meta, now, testIndexTTL))

	rows, err := idx.search(IndexQuery{Text: "quiver-core", Limit: 10}, now)
	require.NoError(t, err)
	require.Len(t, rows, 1, "a hyphen is FTS5 query syntax and must be matched literally")

	for _, q := range []string{`"`, `*`, `chrom OR`, `NEAR(`, `^`} {
		_, err := idx.search(IndexQuery{Text: q, Limit: 10}, now)
		require.NoError(t, err, "query %q must not be parsed as FTS5 syntax", q)
	}
}

// Error paths.

func TestOpenIndex_OpenError(t *testing.T) {
	_, err := openIndex(filepath.Join(t.TempDir(), "missing", "index.db"))
	require.ErrorContains(t, err, "vault index: open")
}

// readOnlyDSN builds a DSN that opens path read-only, so writes fail without
// depending on filesystem permissions (which root would bypass).
func readOnlyDSN(path string) string { return "file:" + path + "?mode=ro" }

func TestOpenIndex_MigrateError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := adapterSQLite.OpenDB(path)
	require.NoError(t, err)
	// Force the file to exist as a valid but unmigrated sqlite database.
	require.NoError(t, db.Exec(`CREATE TABLE seed (x INTEGER)`).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = openIndex(readOnlyDSN(path))
	require.ErrorContains(t, err, "vault index: migrate")
}

func TestOpenIndex_CreateFTSError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	idx, err := openIndex(path)
	require.NoError(t, err)
	// Leave the GORM tables migrated but the FTS table absent, so reopening
	// read-only gets past AutoMigrate and fails on the virtual table DDL.
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrows_fts`).Error)
	require.NoError(t, idx.close())

	_, err = openIndex(readOnlyDSN(path))
	require.ErrorContains(t, err, "vault index: create fts")
}

func TestIndex_Close_InvalidDB(t *testing.T) {
	i := &index{db: &gorm.DB{Config: &gorm.Config{}}}
	require.ErrorContains(t, i.close(), "vault index: close")
}

func TestIndex_Upsert_SQLFailures(t *testing.T) {
	testCases := []struct {
		name    string
		drop    string
		wantErr string
	}{
		{name: "row table missing", drop: "vault_arrows", wantErr: "upsert row"},
		{name: "tag table missing", drop: "vault_arrow_tags", wantErr: "clear tags"},
		{name: "os table missing", drop: "vault_arrow_os", wantErr: "clear os"},
		{name: "fts table missing", drop: "vault_arrows_fts", wantErr: "clear fts"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			idx := newTestIndex(t)
			require.NoError(t, idx.db.Exec("DROP TABLE "+tc.drop).Error)

			err := idx.upsert(
				"github.com/u/r@v1",
				ManifestFile{Filename: "ARROW.md"},
				testMeta(),
				time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
				testIndexTTL,
			)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestReplaceChildRows_WriteTagError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrow_tags`).Error)
	require.NoError(t, idx.db.Exec(`CREATE TABLE vault_arrow_tags (
		namespace TEXT, ref TEXT, tag TEXT CHECK (tag <> 'browser'),
		PRIMARY KEY (namespace, ref, tag))`).Error)

	err := replaceChildRows(idx.db, "github.com/u/r", "v1", testMeta())
	require.ErrorContains(t, err, "write tag")
}

func TestReplaceChildRows_WriteOSError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrow_os`).Error)
	require.NoError(t, idx.db.Exec(`CREATE TABLE vault_arrow_os (
		namespace TEXT, ref TEXT, os TEXT CHECK (os <> 'linux/amd64'),
		PRIMARY KEY (namespace, ref, os))`).Error)

	err := replaceChildRows(idx.db, "github.com/u/r", "v1", testMeta())
	require.ErrorContains(t, err, "write os")
}

func TestReplaceFTS_WriteError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrows_fts`).Error)
	require.NoError(t, idx.db.Exec(`CREATE TABLE vault_arrows_fts (
		namespace TEXT, ref TEXT, name TEXT CHECK (name <> 'Chromium'),
		description TEXT, tags TEXT)`).Error)

	err := replaceFTS(idx.db, "github.com/u/r", "v1", testMeta())
	require.ErrorContains(t, err, "write fts")
}

func TestIndex_Search_QueryError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrows_fts`).Error)

	_, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, time.Now())
	require.ErrorContains(t, err, "vault index: search")
}

func TestIndex_Hydrate_ChildLoadFailures(t *testing.T) {
	testCases := []struct {
		name    string
		drop    string
		wantErr string
	}{
		{name: "tag table missing", drop: "vault_arrow_tags", wantErr: "load tags"},
		{name: "os table missing", drop: "vault_arrow_os", wantErr: "load os"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			idx := newTestIndex(t)
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))
			require.NoError(t, idx.db.Exec("DROP TABLE "+tc.drop).Error)

			_, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestIndex_EvictExpired_SelectError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrows`).Error)

	require.ErrorContains(t, idx.evictExpired(time.Now()), "select expired")
}

func TestIndex_EvictExpired_DeleteError(t *testing.T) {
	idx := newTestIndex(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), t0, 24*time.Hour))
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrow_tags`).Error)

	require.ErrorContains(t, idx.evictExpired(t0.Add(48*time.Hour)), "delete tags")
}

func TestIndex_Forget_SQLFailures(t *testing.T) {
	testCases := []struct {
		name    string
		drop    string
		wantErr string
	}{
		{name: "row table missing", drop: "vault_arrows", wantErr: "forget row"},
		{name: "tag table missing", drop: "vault_arrow_tags", wantErr: "forget tags"},
		{name: "os table missing", drop: "vault_arrow_os", wantErr: "forget os"},
		{name: "fts table missing", drop: "vault_arrows_fts", wantErr: "forget fts"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			idx := newTestIndex(t)
			require.NoError(t, idx.db.Exec("DROP TABLE "+tc.drop).Error)

			require.ErrorContains(t, idx.forget("github.com/u/r"), tc.wantErr)
		})
	}
}

func TestDeleteKey_SQLFailures(t *testing.T) {
	testCases := []struct {
		name    string
		drop    string
		wantErr string
	}{
		{name: "row table missing", drop: "vault_arrows", wantErr: "delete row"},
		{name: "tag table missing", drop: "vault_arrow_tags", wantErr: "delete tags"},
		{name: "os table missing", drop: "vault_arrow_os", wantErr: "delete os"},
		{name: "fts table missing", drop: "vault_arrows_fts", wantErr: "delete fts"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			idx := newTestIndex(t)
			require.NoError(t, idx.db.Exec("DROP TABLE "+tc.drop).Error)

			require.ErrorContains(t, deleteKey(idx.db, "github.com/u/r", "v1"), tc.wantErr)
		})
	}
}

func TestIndex_Upsert_ConcurrentSameKeyConverges(t *testing.T) {
	idx := newTestIndex(t)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	ns := domain.Namespace("github.com/u/r@v1")

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := idx.upsert(ns, ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}
