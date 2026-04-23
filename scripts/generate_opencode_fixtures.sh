#!/usr/bin/env bash
# Generate deterministic fixture SQLite databases for the OpenCode backend tests.
#
# Output:
#   testdata/opencode_simple.db      — 2 linear sessions, a few messages + one completed tool each
#   testdata/opencode_branching.db   — 1 session with a branch (two assistant replies to the same user)
#   testdata/opencode_with_tools.db  — 1 session with all four tool-part statuses (completed, error, running, pending)
#   testdata/opencode_compaction.db  — 1 session with a mid-session compaction boundary
#
# The schema matches OpenCode's drizzle schema at the pinned version (see design §11).
# Rebuild with: bash scripts/generate_opencode_fixtures.sh
#
# Deterministic caveat: SQLite occasionally reorders pages after INSERT. We run
# `VACUUM INTO` at the end to normalize, then move the result over the original.
# Re-running the script should produce byte-identical files on the same OS.

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUT="${ROOT}/testdata"
mkdir -p "${OUT}"

SCHEMA_SQL=$(cat <<'EOF'
CREATE TABLE `project` (
    `id` text PRIMARY KEY,
    `worktree` text NOT NULL,
    `vcs` text,
    `name` text,
    `icon_url` text,
    `icon_color` text,
    `time_created` integer NOT NULL,
    `time_updated` integer NOT NULL,
    `time_initialized` integer,
    `sandboxes` text NOT NULL,
    `commands` text
);
CREATE TABLE `session` (
    `id` text PRIMARY KEY,
    `project_id` text NOT NULL,
    `parent_id` text,
    `slug` text NOT NULL,
    `directory` text NOT NULL,
    `title` text NOT NULL,
    `version` text NOT NULL,
    `share_url` text,
    `summary_additions` integer,
    `summary_deletions` integer,
    `summary_files` integer,
    `summary_diffs` text,
    `revert` text,
    `permission` text,
    `time_created` integer NOT NULL,
    `time_updated` integer NOT NULL,
    `time_compacting` integer,
    `time_archived` integer,
    `workspace_id` text,
    CONSTRAINT `fk_session_project_id_project_id_fk` FOREIGN KEY (`project_id`) REFERENCES `project`(`id`) ON DELETE CASCADE
);
CREATE TABLE `message` (
    `id` text PRIMARY KEY,
    `session_id` text NOT NULL,
    `time_created` integer NOT NULL,
    `time_updated` integer NOT NULL,
    `data` text NOT NULL,
    CONSTRAINT `fk_message_session_id_session_id_fk` FOREIGN KEY (`session_id`) REFERENCES `session`(`id`) ON DELETE CASCADE
);
CREATE TABLE `part` (
    `id` text PRIMARY KEY,
    `message_id` text NOT NULL,
    `session_id` text NOT NULL,
    `time_created` integer NOT NULL,
    `time_updated` integer NOT NULL,
    `data` text NOT NULL,
    CONSTRAINT `fk_part_message_id_message_id_fk` FOREIGN KEY (`message_id`) REFERENCES `message`(`id`) ON DELETE CASCADE
);
CREATE INDEX `part_session_idx` ON `part` (`session_id`);
CREATE INDEX `message_session_time_created_id_idx` ON `message` (`session_id`,`time_created`,`id`);
CREATE INDEX `part_message_id_id_idx` ON `part` (`message_id`,`id`);
EOF
)

# project row used by every fixture. Fixed ID keeps joins predictable.
PROJECT_ROW='INSERT INTO project (id, worktree, vcs, name, time_created, time_updated, sandboxes)
             VALUES ("prj_1", "/home/user/code", "git", "code", 1700000000000, 1700000000000, "{}");'

# Helper: build a fresh DB at $1 with the shared schema + project row.
make_db() {
    local path="$1"
    rm -f "${path}"
    sqlite3 "${path}" <<SQL
PRAGMA foreign_keys = ON;
${SCHEMA_SQL}
${PROJECT_ROW}
SQL
}

# ── simple.db ─────────────────────────────────────────────────────────────
SIMPLE="${OUT}/opencode_simple.db"
make_db "${SIMPLE}"
sqlite3 "${SIMPLE}" <<'SQL'
-- Session A: one user → one assistant with text part, one user → one assistant with a completed tool part.
INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
VALUES ("ses_s1", "prj_1", "simple-a", "/home/user/code", "Simple A", "0.1.0", 1700001000000, 1700001050000);

-- Session B: two-turn linear chat.
INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
VALUES ("ses_s2", "prj_1", "simple-b", "/home/user/code", "Simple B", "0.1.0", 1700002000000, 1700002050000);

-- ses_s1 messages
INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
  ("msg_u1", "ses_s1", 1700001000000, 1700001000000,
   '{"role":"user","time":{"created":1700001000000}}'),
  ("msg_a1", "ses_s1", 1700001001000, 1700001001000,
   '{"role":"assistant","parentID":"msg_u1","tokens":{"input":10,"output":20,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0.001}'),
  ("msg_u2", "ses_s1", 1700001040000, 1700001040000,
   '{"role":"user","time":{"created":1700001040000}}'),
  ("msg_a2", "ses_s1", 1700001050000, 1700001050000,
   '{"role":"assistant","parentID":"msg_u2","tokens":{"input":15,"output":30,"reasoning":5,"cache":{"read":100,"write":0}},"cost":0.002}');

INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
  ("prt_u1a", "msg_u1", "ses_s1", 1700001000000, 1700001000000,
   '{"type":"text","text":"hello"}'),
  ("prt_a1a", "msg_a1", "ses_s1", 1700001001000, 1700001001000,
   '{"type":"text","text":"hi there"}'),
  ("prt_u2a", "msg_u2", "ses_s1", 1700001040000, 1700001040000,
   '{"type":"text","text":"run ls"}'),
  ("prt_a2a", "msg_a2", "ses_s1", 1700001050000, 1700001050000,
   '{"type":"tool","callID":"call_1","tool":"bash","state":{"status":"completed","input":{"cmd":"ls"},"output":"file1\nfile2"}}');

-- ses_s2 messages
INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
  ("msg_s2u1", "ses_s2", 1700002000000, 1700002000000,
   '{"role":"user"}'),
  ("msg_s2a1", "ses_s2", 1700002010000, 1700002010000,
   '{"role":"assistant","parentID":"msg_s2u1","tokens":{"input":8,"output":12,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0.0005}');

INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
  ("prt_s2u1a", "msg_s2u1", "ses_s2", 1700002000000, 1700002000000,
   '{"type":"text","text":"what time is it"}'),
  ("prt_s2a1a", "msg_s2a1", "ses_s2", 1700002010000, 1700002010000,
   '{"type":"text","text":"I cannot tell the time"}');
SQL

# ── branching.db ──────────────────────────────────────────────────────────
# user → assistant A (old branch) vs user → assistant B (newer leaf).
# The walker must pick B since its time_created is later.
BRANCHING="${OUT}/opencode_branching.db"
make_db "${BRANCHING}"
sqlite3 "${BRANCHING}" <<'SQL'
-- Session time_updated is 90s past time_created to span the regen gap.
INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
VALUES ("ses_br", "prj_1", "branching", "/home/user/code", "Branching", "0.1.0", 1700003000000, 1700003180000);

-- Gap between the two assistant replies is 3 minutes (>> 60s threshold) so
-- markRegeneratedBranches drops msg_ba_old.
INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
  ("msg_bu1", "ses_br", 1700003000000, 1700003000000,
   '{"role":"user"}'),
  -- first (older) assistant reply; will be DROPPED as superseded regen.
  ("msg_ba_old", "ses_br", 1700003005000, 1700003005000,
   '{"role":"assistant","parentID":"msg_bu1","tokens":{"input":5,"output":10,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0.001}'),
  -- regen: newer assistant reply 3 minutes later; this is the current branch.
  ("msg_ba_new", "ses_br", 1700003180000, 1700003180000,
   '{"role":"assistant","parentID":"msg_bu1","tokens":{"input":5,"output":15,"reasoning":2,"cache":{"read":0,"write":0}},"cost":0.002}');

INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
  ("prt_bu1a", "msg_bu1", "ses_br", 1700003000000, 1700003000000,
   '{"type":"text","text":"pick a number"}'),
  ("prt_ba_old_a", "msg_ba_old", "ses_br", 1700003005000, 1700003005000,
   '{"type":"text","text":"OLD: 7"}'),
  ("prt_ba_new_a", "msg_ba_new", "ses_br", 1700003180000, 1700003180000,
   '{"type":"text","text":"NEW: 42"}');
SQL

# ── with_tools.db ─────────────────────────────────────────────────────────
# One session whose assistant turns have one tool part per status:
#   completed, error, running, pending.
TOOLS="${OUT}/opencode_with_tools.db"
make_db "${TOOLS}"
sqlite3 "${TOOLS}" <<'SQL'
INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
VALUES ("ses_tl", "prj_1", "tools", "/home/user/code", "Tools", "0.1.0", 1700004000000, 1700004040000);

INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
  ("msg_tu1", "ses_tl", 1700004000000, 1700004000000, '{"role":"user"}'),
  ("msg_ta1", "ses_tl", 1700004010000, 1700004010000,
   '{"role":"assistant","parentID":"msg_tu1","tokens":{"input":20,"output":40,"reasoning":10,"cache":{"read":0,"write":0}},"cost":0.003}'),
  ("msg_tu2", "ses_tl", 1700004020000, 1700004020000, '{"role":"user"}'),
  ("msg_ta2", "ses_tl", 1700004040000, 1700004040000,
   '{"role":"assistant","parentID":"msg_tu2","tokens":{"input":25,"output":50,"reasoning":15,"cache":{"read":0,"write":0}},"cost":0.004}');

-- First assistant turn: completed + error.
-- Second assistant turn: running + pending.
INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
  ("prt_tu1a", "msg_tu1", "ses_tl", 1700004000000, 1700004000000,
   '{"type":"text","text":"check disk"}'),
  ("prt_ta1_ok", "msg_ta1", "ses_tl", 1700004010001, 1700004010001,
   '{"type":"tool","callID":"call_c","tool":"bash","state":{"status":"completed","input":{"cmd":"df -h"},"output":"Filesystem     Size"}}'),
  ("prt_ta1_err", "msg_ta1", "ses_tl", 1700004010002, 1700004010002,
   '{"type":"tool","callID":"call_e","tool":"bash","state":{"status":"error","input":{"cmd":"asdf"},"output":"command not found: asdf"}}'),
  ("prt_tu2a", "msg_tu2", "ses_tl", 1700004020000, 1700004020000,
   '{"type":"text","text":"keep going"}'),
  ("prt_ta2_run", "msg_ta2", "ses_tl", 1700004040001, 1700004040001,
   '{"type":"tool","callID":"call_r","tool":"bash","state":{"status":"running","input":{"cmd":"sleep 30"}}}'),
  ("prt_ta2_pen", "msg_ta2", "ses_tl", 1700004040002, 1700004040002,
   '{"type":"tool","callID":"call_p","tool":"bash","state":{"status":"pending","input":{"cmd":"echo queued"}}}');
SQL

# ── compaction.db ─────────────────────────────────────────────────────────
# Four turns; a compaction boundary sits on the assistant turn at index 1.
COMPACT="${OUT}/opencode_compaction.db"
make_db "${COMPACT}"
sqlite3 "${COMPACT}" <<'SQL'
INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
VALUES ("ses_cp", "prj_1", "compaction", "/home/user/code", "Compaction", "0.1.0", 1700005000000, 1700005050000);

INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
  ("msg_cu1", "ses_cp", 1700005000000, 1700005000000, '{"role":"user"}'),
  ("msg_ca1", "ses_cp", 1700005010000, 1700005010000,
   '{"role":"assistant","parentID":"msg_cu1","tokens":{"input":10,"output":20,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0.001}'),
  ("msg_cu2", "ses_cp", 1700005020000, 1700005020000, '{"role":"user"}'),
  ("msg_ca2", "ses_cp", 1700005050000, 1700005050000,
   '{"role":"assistant","parentID":"msg_cu2","tokens":{"input":12,"output":24,"reasoning":0,"cache":{"read":50,"write":0}},"cost":0.002}');

-- Compaction part is attached to the second assistant message at the start;
-- decodeChain emits the boundary at the current turn index (2 = the third
-- emitted turn will be the first post-compaction turn).
INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
  ("prt_cu1a", "msg_cu1", "ses_cp", 1700005000000, 1700005000000,
   '{"type":"text","text":"hi"}'),
  ("prt_ca1a", "msg_ca1", "ses_cp", 1700005010000, 1700005010000,
   '{"type":"text","text":"hello"}'),
  ("prt_cu2a", "msg_cu2", "ses_cp", 1700005020000, 1700005020000,
   '{"type":"text","text":"continue"}'),
  ("prt_ca2_compact", "msg_ca2", "ses_cp", 1700005050000, 1700005050000,
   '{"type":"compaction","auto":false}'),
  ("prt_ca2_text", "msg_ca2", "ses_cp", 1700005050001, 1700005050001,
   '{"type":"text","text":"resumed"}');
SQL

# Vacuum for byte-stable output.
for db in "${SIMPLE}" "${BRANCHING}" "${TOOLS}" "${COMPACT}"; do
    tmp="${db}.tmp"
    sqlite3 "${db}" "VACUUM INTO '${tmp}';"
    mv "${tmp}" "${db}"
done

echo "Generated:"
ls -lh "${OUT}"/opencode_*.db
