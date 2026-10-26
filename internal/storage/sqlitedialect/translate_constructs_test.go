package sqlitedialect

import "testing"

// One row per MySQL-ism that today's issueops/sqlbuild emit (v1.3.1). Exact match,
// so a change in output shape is a conscious edit here.
func TestTranslateConstructs(t *testing.T) {
	cases := []struct{ name, in, want string }{
		// CONCAT
		{"concat plain", "SELECT CONCAT(',', ?, ',', issue_id, ',')", "SELECT (',' || ? || ',' || issue_id || ',')"},
		{"concat in LIKE", "WHERE id LIKE CONCAT(?, '-%')", "WHERE id LIKE (? || '-%')"},
		{"concat nested", "CONCAT(d.path, CONCAT(a, b), ',')", "(d.path || (a || b) || ',')"},
		{"concat comma inside string kept", "CONCAT('a,b', x)", "('a,b' || x)"},
		// LOCATE (args swap)
		{"locate", "LOCATE(CONCAT(',', e.issue_id, ','), d.path) = 0", "INSTR(d.path, (',' || e.issue_id || ',')) = 0"},
		// time
		{"utc_timestamp", "defer_until <= UTC_TIMESTAMP()", "defer_until <= CURRENT_TIMESTAMP"},
		{"now lower", "VALUES (?, now())", "VALUES (?, CURRENT_TIMESTAMP)"},
		{"now inside identifier untouched", "SELECT FOO_NOW()", "SELECT FOO_NOW()"},
		{"now in string untouched", "SELECT 'NOW()'", "SELECT 'NOW()'"},
		{"now in comment untouched", "SELECT 1 -- NOW()", "SELECT 1 -- NOW()"},
		// JSON
		{"json_arrayagg", "SELECT issue_id, JSON_ARRAYAGG(label) AS labels_json FROM labels", "SELECT issue_id, json_group_array(label) AS labels_json FROM labels"},
		{"json_arrayagg of json_object", "JSON_ARRAYAGG(JSON_OBJECT('a', x))", "json_group_array(JSON_OBJECT('a', x))"},
		{"json_extract native", "JSON_EXTRACT(metadata, ?) IS NOT NULL", "JSON_EXTRACT(metadata, ?) IS NOT NULL"},
		{"json_unquote(json_extract)", "COALESCE(JSON_UNQUOTE(JSON_EXTRACT(d.metadata, '$.gate')), 'all-children') = 'any-children'", "COALESCE(JSON_EXTRACT(d.metadata, '$.gate'), 'all-children') = 'any-children'"},
		{"json_unquote(json_extract) with placeholder", "JSON_UNQUOTE(JSON_EXTRACT(metadata, ?)) = ?", "JSON_EXTRACT(metadata, ?) = ?"},
		// DATE_FORMAT
		{"date_format iso", "DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%sZ')", "strftime('%Y-%m-%dT%H:%M:%SZ', created_at)"},
		{"date_format date only", "DATE_FORMAT(ts, '%Y-%m-%d')", "strftime('%Y-%m-%d', ts)"},
		{"date_format %T", "DATE_FORMAT(ts, '%T')", "strftime('%H:%M:%S', ts)"},
		{"date_format unsupported month name stays loud", "DATE_FORMAT(ts, '%M')", "DATE_FORMAT(ts, '%M')"},
		{"date_format non-literal stays loud", "DATE_FORMAT(ts, ?)", "DATE_FORMAT(ts, ?)"},
		// INSERT IGNORE / REPLACE
		{"insert ignore", "INSERT IGNORE INTO labels (issue_id, label) VALUES (?, ?)", "INSERT OR IGNORE INTO labels (issue_id, label) VALUES (?, ?)"},
		{"insert ignore lower multi-space", "insert  ignore\n into t (a) VALUES (?)", "INSERT OR IGNORE INTO t (a) VALUES (?)"},
		{"replace into native", "REPLACE INTO config (`key`, value) VALUES (?, ?)", "REPLACE INTO config (`key`, value) VALUES (?, ?)"},
		// ON DUPLICATE KEY UPDATE
		{"odk config", "INSERT INTO config (`key`, value) VALUES (?, ?)\n\t\tON DUPLICATE KEY UPDATE value = VALUES(value)", "INSERT INTO config (`key`, value) VALUES (?, ?) ON CONFLICT (`key`) DO UPDATE SET value = excluded.value"},
		{"odk metadata", "INSERT INTO metadata (`key`, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)", "INSERT INTO metadata (`key`, value) VALUES (?, ?) ON CONFLICT (`key`) DO UPDATE SET value = excluded.value"},
		{"odk child_counters", "INSERT INTO child_counters (parent_id, last_child) VALUES (?, ?)\n ON DUPLICATE KEY UPDATE last_child = ?", "INSERT INTO child_counters (parent_id, last_child) VALUES (?, ?) ON CONFLICT (parent_id) DO UPDATE SET last_child = ?"},
		{"odk wisp_child_counters greatest", "INSERT INTO wisp_child_counters (parent_id, last_child) VALUES (?, ?) ON DUPLICATE KEY UPDATE last_child = GREATEST(wisp_child_counters.last_child, ?)", "INSERT INTO wisp_child_counters (parent_id, last_child) VALUES (?, ?) ON CONFLICT (parent_id) DO UPDATE SET last_child = max(wisp_child_counters.last_child, ?)"},
		{"odk repo_mtimes now", "INSERT INTO repo_mtimes (repo_path, jsonl_path, mtime_ns, last_checked) VALUES (?, ?, ?, NOW()) ON DUPLICATE KEY UPDATE last_checked = NOW()", "INSERT INTO repo_mtimes (repo_path, jsonl_path, mtime_ns, last_checked) VALUES (?, ?, ?, CURRENT_TIMESTAMP) ON CONFLICT (repo_path) DO UPDATE SET last_checked = CURRENT_TIMESTAMP"},
		{"odk federation_peers", "INSERT INTO federation_peers (name, remote_url) VALUES (?, ?) ON DUPLICATE KEY UPDATE remote_url = VALUES(remote_url)", "INSERT INTO federation_peers (name, remote_url) VALUES (?, ?) ON CONFLICT (name) DO UPDATE SET remote_url = excluded.remote_url"},
		{"odk issues plain", "INSERT INTO issues (id, title) VALUES (?, ?) ON DUPLICATE KEY UPDATE title = VALUES(title)", "INSERT INTO issues (id, title) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET title = excluded.title"},
		{"odk wisps stale guard", "INSERT INTO wisps (id, title, updated_at) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE title = IF(VALUES(updated_at) > wisps.updated_at, VALUES(title), wisps.title)", "INSERT INTO wisps (id, title, updated_at) VALUES (?, ?, ?) ON CONFLICT (id) DO UPDATE SET title = CASE WHEN excluded.updated_at > wisps.updated_at THEN excluded.title ELSE wisps.title END"},
		{"odk self-assign (events type=type)", "INSERT INTO events (id, type) VALUES (?, ?) ON DUPLICATE KEY UPDATE type = type", "INSERT INTO events (id, type) VALUES (?, ?) ON CONFLICT DO NOTHING"},
		{"odk unknown table stays loud", "INSERT INTO nosuch (a) VALUES (?) ON DUPLICATE KEY UPDATE a = VALUES(a)", "INSERT INTO nosuch (a) VALUES (?) ON CONFLICT () DO UPDATE SET a = excluded.a"},
		// IF outside an upsert
		{"if in update", "UPDATE leases SET granted_node = IF(COALESCE(granted_node, '') = '', ?, granted_node) WHERE issue_id = ?", "UPDATE leases SET granted_node = CASE WHEN COALESCE(granted_node, '') = '' THEN ? ELSE granted_node END WHERE issue_id = ?"},
		{"if not exists untouched", "CREATE TABLE IF NOT EXISTS t (a int)", "CREATE TABLE IF NOT EXISTS t (a int)"},
		// GREATEST / LEAST
		{"greatest", "next_seq = GREATEST(next_seq, COALESCE((SELECT MAX(seq) FROM j), 0))", "next_seq = max(next_seq, COALESCE((SELECT MAX(seq) FROM j), 0))"},
		{"least", "LEAST(a, b, c)", "min(a, b, c)"},
		{"greatest single arg stays loud", "GREATEST(a)", "GREATEST(a)"},
		// <=>
		{"null-safe eq", "AND old_value <=> ? AND new_value <=> ? AND comment <=> ?", "AND old_value IS ? AND new_value IS ? AND comment IS ?"},
		{"null-safe eq in string untouched", "SELECT '<=>'", "SELECT '<=>'"},
		// FOR UPDATE
		{"for update", "SELECT value FROM config WHERE `key` = ? FOR UPDATE", "SELECT value FROM config WHERE `key` = ?"},
		{"for update lower", "SELECT 1 for update", "SELECT 1"},
		{"lock in share mode", "SELECT 1 LOCK IN SHARE MODE", "SELECT 1"},
		{"for update in comment untouched", "SELECT 1 /* for update */", "SELECT 1 /* for update */"},
		// UPDATE alias / set targets
		{"update alias", "UPDATE issues i SET i.is_blocked = 1 WHERE i.id = ?", "UPDATE issues AS i SET is_blocked = 1 WHERE i.id = ?"},
		{"update no alias", "UPDATE issues SET title = ? WHERE id = ?", "UPDATE issues SET title = ? WHERE id = ?"},
		// label ordering
		{"order by label", "SELECT label FROM labels ORDER BY label", "SELECT label FROM labels ORDER BY label COLLATE BINARY"},
		// already-portable forms pass through
		{"instr/substring/cast native", "INSTR(SUBSTRING(id, LENGTH(?) + 2), '.') = 0 AND CAST(ts AS CHAR) <> ''", "INSTR(SUBSTRING(id, LENGTH(?) + 2), '.') = 0 AND CAST(ts AS CHAR) <> ''"},
		{"json_object native", "JSON_OBJECT('a', 1)", "JSON_OBJECT('a', 1)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Translate(tc.in); got != tc.want {
				t.Errorf("Translate(%q)\n got  %q\n want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Translate must be stable: running it on its own output changes nothing.
func TestTranslateIdempotent(t *testing.T) {
	for _, in := range []string{
		"INSERT INTO config (`key`, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)",
		"SELECT CONCAT(a, b), LOCATE(x, y), IF(a, b, c), DATE_FORMAT(t, '%Y') FROM t FOR UPDATE",
		"INSERT IGNORE INTO t (a) VALUES (NOW())",
	} {
		once := Translate(in)
		if twice := Translate(once); twice != once {
			t.Errorf("not idempotent:\n once  %q\n twice %q", once, twice)
		}
	}
}
