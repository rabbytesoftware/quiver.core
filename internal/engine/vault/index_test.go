package vault

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
			License:     "BSD-3-Clause",
			URL:         "https://www.chromium.org",
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
// list untouched; only a deliberate schema change may edit it.
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
				"namespace", "ref", "name", "description", "license", "url",
				"icon", "banner", "stars", "source", "filename", "branch",
				"seen_at", "row_expire_at", "generator", "confidence",
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
	require.Equal(t, "BSD-3-Clause", got.Meta.Arrow.License)
	require.Equal(t, "https://www.chromium.org", got.Meta.Arrow.URL)
	require.Equal(t, []string{"browser", "web"}, got.Meta.Arrow.Tags)
	require.Equal(t, "icon.png", got.Meta.Arrow.Media.Icon)
	require.Equal(t, "banner.png", got.Meta.Arrow.Media.Banner)
	require.Equal(t, []domain.OS{domain.OSLinuxAMD64}, got.Meta.OS)
	require.Equal(t, 42, got.Meta.Stars)
	require.Equal(t, "github.com", got.Meta.Source)
	require.True(t, got.SeenAt.Equal(now))
}

func TestIndex_Search_RoundTripsGenerator(t *testing.T) {
	testCases := []struct {
		name      string
		generator *domain.ArrowGenerator
	}{
		{name: "declared arrow has no generator", generator: nil},
		{
			name:      "inferred arrow",
			generator: &domain.ArrowGenerator{Name: "fletcher/1", Confidence: "high"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			idx := newTestIndex(t)
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			meta := testMeta()
			meta.Arrow.Generator = tc.generator
			require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, meta, now, testIndexTTL))

			rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, tc.generator, rows[0].Meta.Arrow.Generator)
		})
	}
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
	idx, err := openIndex(filepath.Join(t.TempDir(), "missing", "index.db"))
	require.Error(t, err)
	assert.Nil(t, idx)
}

// readOnlyDSN builds a DSN that opens path read-only, so writes fail without
// depending on filesystem permissions (which root would bypass).
func readOnlyDSN(path string) string { return "file:" + path + "?mode=ro" }

func indexTables() []string {
	return []string{"vault_arrows", "vault_arrow_tags", "vault_arrow_os", "vault_arrows_fts"}
}

func tableRows(
	t *testing.T,
	idx *index,
	table string,
) int64 {
	t.Helper()
	var n int64
	require.NoError(t, idx.db.Raw("SELECT COUNT(*) FROM "+table).Scan(&n).Error)
	return n
}

func seedIndex(
	t *testing.T,
	idx *index,
) {
	t.Helper()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))
}

func assertSeedIntact(
	t *testing.T,
	idx *index,
	dropped string,
) {
	t.Helper()
	want := map[string]int64{"vault_arrows": 1, "vault_arrow_tags": 2, "vault_arrow_os": 1, "vault_arrows_fts": 1}
	for _, table := range indexTables() {
		if table != dropped {
			assert.Equal(t, want[table], tableRows(t, idx, table), table)
		}
	}
}

func TestOpenIndex_MigrateError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := adapterSQLite.OpenDB(path)
	require.NoError(t, err)
	// Force the file to exist as a valid but unmigrated sqlite database.
	require.NoError(t, db.Exec(`CREATE TABLE seed (x INTEGER)`).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	idx, err := openIndex(readOnlyDSN(path))
	require.Error(t, err)
	assert.Nil(t, idx)
}

func TestOpenIndex_CreateFTSError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	idx, err := openIndex(path)
	require.NoError(t, err)
	// Leave the GORM tables migrated but the FTS table absent, so reopening
	// read-only gets past AutoMigrate and fails on the virtual table DDL.
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrows_fts`).Error)
	require.NoError(t, idx.close())

	reopened, err := openIndex(readOnlyDSN(path))
	require.Error(t, err)
	assert.Nil(t, reopened)
}

func TestIndex_Close_InvalidDB(t *testing.T) {
	i := &index{db: &gorm.DB{Config: &gorm.Config{}}}
	require.Error(t, i.close())
}

func TestIndex_Upsert_SQLFailuresRollBack(t *testing.T) {
	for _, dropped := range indexTables() {
		t.Run(dropped, func(t *testing.T) {
			idx := newTestIndex(t)
			require.NoError(t, idx.db.Exec("DROP TABLE "+dropped).Error)

			err := idx.upsert(
				"github.com/u/r@v1",
				ManifestFile{Filename: "ARROW.md"},
				testMeta(),
				time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
				testIndexTTL,
			)
			require.Error(t, err)
			for _, table := range indexTables() {
				if table != dropped {
					assert.Zero(t, tableRows(t, idx, table), table)
				}
			}
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
	require.Error(t, err)
	assert.Zero(t, tableRows(t, idx, "vault_arrow_tags"))
	assert.Zero(t, tableRows(t, idx, "vault_arrow_os"), "writing stops at the first failed tag")
}

func TestReplaceChildRows_WriteOSError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrow_os`).Error)
	require.NoError(t, idx.db.Exec(`CREATE TABLE vault_arrow_os (
		namespace TEXT, ref TEXT, os TEXT CHECK (os <> 'linux/amd64'),
		PRIMARY KEY (namespace, ref, os))`).Error)

	err := replaceChildRows(idx.db, "github.com/u/r", "v1", testMeta())
	require.Error(t, err)
	assert.Equal(t, int64(2), tableRows(t, idx, "vault_arrow_tags"), "tags are written before the os rows")
	assert.Zero(t, tableRows(t, idx, "vault_arrow_os"))
}

func TestReplaceFTS_WriteError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrows_fts`).Error)
	require.NoError(t, idx.db.Exec(`CREATE TABLE vault_arrows_fts (
		namespace TEXT, ref TEXT, name TEXT CHECK (name <> 'Chromium'),
		description TEXT, tags TEXT)`).Error)

	err := replaceFTS(idx.db, "github.com/u/r", "v1", testMeta())
	require.Error(t, err)
	assert.Zero(t, tableRows(t, idx, "vault_arrows_fts"))
}

func TestIndex_Search_QueryError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrows_fts`).Error)

	rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, time.Now())
	require.Error(t, err)
	assert.Nil(t, rows)
}

func TestIndex_Hydrate_ChildLoadFailures(t *testing.T) {
	for _, dropped := range []string{"vault_arrow_tags", "vault_arrow_os"} {
		t.Run(dropped, func(t *testing.T) {
			idx := newTestIndex(t)
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), now, testIndexTTL))
			require.NoError(t, idx.db.Exec("DROP TABLE "+dropped).Error)

			rows, err := idx.search(IndexQuery{Text: "chrom", Limit: 10}, now)
			require.Error(t, err)
			assert.Nil(t, rows)
		})
	}
}

func TestIndex_EvictExpired_SelectError(t *testing.T) {
	idx := newTestIndex(t)
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrows`).Error)

	require.Error(t, idx.evictExpired(time.Now()))
}

func TestIndex_EvictExpired_DeleteErrorRollsBack(t *testing.T) {
	idx := newTestIndex(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, idx.upsert("github.com/u/r@v1", ManifestFile{Filename: "ARROW.md"}, testMeta(), t0, 24*time.Hour))
	require.NoError(t, idx.db.Exec(`DROP TABLE vault_arrow_tags`).Error)

	require.Error(t, idx.evictExpired(t0.Add(48*time.Hour)))
	assert.Equal(t, int64(1), tableRows(t, idx, "vault_arrows"))
}

func TestIndex_Forget_SQLFailuresRollBack(t *testing.T) {
	for _, dropped := range indexTables() {
		t.Run(dropped, func(t *testing.T) {
			idx := newTestIndex(t)
			seedIndex(t, idx)
			require.NoError(t, idx.db.Exec("DROP TABLE "+dropped).Error)

			require.Error(t, idx.forget("github.com/u/r"))
			assertSeedIntact(t, idx, dropped)
		})
	}
}

func TestDeleteKey_SQLFailuresRollBack(t *testing.T) {
	for _, dropped := range indexTables() {
		t.Run(dropped, func(t *testing.T) {
			idx := newTestIndex(t)
			seedIndex(t, idx)
			require.NoError(t, idx.db.Exec("DROP TABLE "+dropped).Error)

			err := idx.db.Transaction(func(tx *gorm.DB) error {
				return deleteKey(tx, "github.com/u/r", "v1")
			})
			require.Error(t, err)
			assertSeedIntact(t, idx, dropped)
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

// A manifest may repeat a tag: no ruleset forbids it, and the catalog read model
// already tolerates it. The index keys tag rows by (namespace, ref, tag), so
// writing the raw list failed the whole upsert — which discovery reports as an
// arrow that could not be verified, hiding a perfectly good one.
func TestIndex_Upsert_DuplicateTagInManifest(t *testing.T) {
	idx := newTestIndex(t)

	meta := testMeta()
	meta.Arrow.Tags = []string{"browser", "browser", "web"}

	require.NoError(t, idx.upsert(
		domain.Namespace("github.com/acme/chromium@v1"),
		ManifestFile{Filename: "arrow.yaml"},
		meta, time.Now(), testIndexTTL,
	))

	rows, err := idx.search(IndexQuery{Text: "Chromium"}, time.Now())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, []string{"browser", "web"}, rows[0].Meta.Arrow.Tags,
		"a repeated tag must be stored once, not fail the write")
}
