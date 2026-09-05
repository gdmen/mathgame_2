"""Generate docs/architecture.drawio, the repo architecture diagram.

Run from the repo root: python3 scripts/gen_architecture_diagram.py
Open the output at https://app.diagrams.net (or any draw.io editor). A clickable
link page is also written to .context/diagram.html when that directory exists.
"""
import json, zlib, base64, os
DRAWIO_OUT = "docs/architecture.drawio"
VIEWER_OUT = "docs/architecture.html"
HTML_OUT = ".context/diagram.html"
from urllib.parse import quote
from xml.sax.saxutils import escape as _sax_escape
def escape(s):
    return _sax_escape(s, {'"': '&quot;'})

LAYERS = {
    "client": ("#dae8fc", "#6c8ebf", "#f4f8fe"),
    "server": ("#ffe6cc", "#d79b00", "#fff7ee"),
    "pipeline": ("#d5e8d4", "#82b366", "#f3f9f2"),
    "funnel": ("#fff2cc", "#d6b656", "#fffbea"),
    "table": ("#e1d5e7", "#9673a6", "#faf6fc"),
    "join": ("#f5f5f5", "#666666", "#ffffff"),
    "jobs": ("#f8cecc", "#b85450", "#fdf1f0"),
}

E = {}
def ent(name, layer, rows, w=460, idx=None):
    E[name] = dict(name=name, layer=layer, rows=rows, w=w, idx=idx or [])

# --- Client (React SPA, web/src) ---------------------------------------------------
ent("APP_SHELL (index.js, auth0.js)", "client", [
    ("AppView", "Auth0Provider; getAccessTokenSilently -> Bearer token on every call"),
    ("routes", "/login /play /settings /progress /admin/* /pin/:redirect_pathname; * -> NotFound"),
    ("refreshPageLoadData", "GET /pageload/{auth0_id}; 404 -> POST /users (auto-provision)"),
    ("postEvent", "POST /events {event_type, value}"),
    ("useTakeover", "setup wizard when user.pin == \"\"; VideosRepairView when numEnabledVideos < MIN_PLAYABLE_VIDEOS"),
])
ent("PLAY_VIEW (play.js)", "client", [
    ("getPlayData", "GET /play/{user_id} -> {gamestate, problem, video}; 403 -> blocked"),
    ("ProblemView", "KaTeX render; WORKING_ON_PROBLEM every interval while focused"),
    ("AnswerTracker", "ANSWERED_PROBLEM; reply gamestate swaps in next problem in place"),
    ("VideoView", "react-player (youtube-nocookie): WATCHING_VIDEO, DONE_WATCHING_VIDEO, ERROR_PLAYING_VIDEO"),
    ("skip / report", "PinConfirmModal -> BAD_PROBLEM_USER; KaTeX throw -> BAD_PROBLEM_SYSTEM"),
])
ent("SETTINGS_SETUP (settings.js, setup.js, pin.js)", "client", [
    ("RequirePin", "sessionStorage \"math-game-pin\"; PIN_PROTECTED_PATHS = [\"/settings\"]"),
    ("postSettings", "POST /settings/{user_id} (bitmap, target_difficulty, work %)"),
    ("playlists", "GET/POST/DELETE /playlists; GET /playlists/{id}/videos (read-only)"),
    ("SetupView", "tabs: Problem Types / Add Videos / Set Parent Pin / Start Playing!"),
    ("DeleteAccountView", "DELETE /users/{auth0_id} {pin}"),
])
ent("PROGRESS_ADMIN (progress.js, admin_*.js)", "client", [
    ("ProgressView", "GET /statistics/{user_id}"),
    ("DifficultyCalibrationView", "GET/POST /admin/difficulty-calibration(/recompute)"),
    ("BitmapMatrixView", "GET/POST /admin/bitmap-matrix(/recompute); GET .../cell"),
    ("ApiDocsView", "Swagger UI over /admin/swagger.yaml"),
])

# --- Edge / server -----------------------------------------------------------------
ent("EXTERNAL_SERVICES", "join", [
    ("Auth0", "SPA login + JWT verify; Management API user delete (delete:users)"),
    ("OpenAI", "GPT5Nano NarrateProblems; GPT5 ValidateWordProblem"),
    ("YouTube Data API v3", "playlists, playlistItems, videos + oembed fallback"),
    ("ntfy.sh", "watchdog alerts (ntfy_topic)"),
])
ent("NGINX (deploy/nginx/mikeymath.conf)", "server", [
    ("mikeymath.org :443", "root /var/www/mathgame/build; SPA routes -> try_files /app.html"),
    ("location ^~ /api/", "proxy_pass http://127.0.0.1:8080; proxy_read_timeout 300s"),
    ("maintenance.on", "-> 503 @maintenance, Retry-After: 120"),
])
ent("APISERVER (cmd/apiserver, server/api)", "server", [
    ("middleware", "cors -> RequestIdMiddleware -> auth0.EnsureValidToken -> Auth0IdMiddleware; RequireSelf / RequireAdmin"),
    ("GET /pageload/:auth0_id", "customGetPageLoadData"),
    ("GET /play/:user_id", "customGetPlayData; selectProblem only when the stored problem is missing / not active"),
    ("POST /events", "customCreateEvent -> processEvents"),
    ("POST /settings/:user_id", "customUpdateSettings"),
    ("playlists CRUD", "customAddPlaylist / customListPlaylistVideos -> YOUTUBE_SYNC"),
    ("users", "customCreateOrUpdateUser; customDeleteAccount -> Auth0 mgmt, purges 9 per-user tables, anonymizes users + events"),
    ("GET /statistics/:user_id", "getStatistics -> UpdateStatisticsForUser"),
    ("/admin/*", "adminDifficultyCalibration, adminBitmapMatrix, adminSwaggerSpec"),
    ("startup", "RunMigrations; binds 127.0.0.1:api_port in ReleaseMode"),
])
ent("YOUTUBE_SYNC (youtube.go, custom_handlers.go)", "server", [
    ("syncPlaylistFromYouTube", "YouTube Data API v3 -> playlists, videos, playlist_video"),
    ("refreshUserHasVideo", "rebuilds user_has_video from user_playlist x playlist_video after every playlist change"),
    ("selectVideo", "random from user_has_video join videos where disabled = 0"),
])

# --- Domain pipelines & jobs -------------------------------------------------------
ent("selectProblem (generate_problems.go)", "funnel", [
    ("[1] getDueReviewProblem", "serve due review_queue problem if still active"),
    ("[2] getSatisfyingProblemIds", "bitmap subset + difficulty +/- epsilon + status='active', minus recently shown; newestVersionTier by generatorRank"),
    ("[3] pool top-up", "pool < SelectionPoolCap & WORD bit -> generateProblemsBackground (per-user TryLock)"),
    ("[4] pickWithRecencyBias", "select_lru.go: uniform pick among least-recently-shown fraction"),
    ("[5] empty pool", "live runHeuristicGenerator(bitmap &^ WORD)"),
    ("[6] relax recency", "retry without prevIds exclusion"),
    ("[7] last resort", "blocking generateProblem, WORD-only envelope (5 x 5 retries)"),
])
ent("generateProblems (generate_problems.go)", "pipeline", [
    ("runHeuristicGenerator", "non-WORD: BuildProblem -> ADMISSION_FUNNEL -> Generator = heuristic_generator.VERSION"),
    ("runWordGenerator", "WORD: BuildWordSkeletonRaw -> NarrateProblems -> ADMISSION_FUNNEL (prose + symbolic) -> Generator = llm_generator.VERSION"),
    ("test seams", "llmNarrateProblemFn / llmValidateProblemFn package vars"),
])
ent("ADMISSION_FUNNEL", "funnel", [
    ("[0] NormalizeExpression", "times, frac, div, unicode, money, thousands separators"),
    ("[1] LexExpression", "reject: lexer"),
    ("[1.5] RewriteLoneVariable", "bare letter -> ?; reduceLabeledUnknown"),
    ("[2] DetectProblemTypeBitmap", "stamp bits; [2.5] reject: unknown_rules"),
    ("[3] VerifyAnswerSymbolic", "exact big.Rat eval; WORD also runs the LLM validator; reject: answer / validator"),
    ("[3.5] EnvelopeViolation", "stamped bits must be a subset of the user bitmap; reject: envelope"),
    ("ComputeProblemDifficulty", "stamps difficulty + difficulty_version 0.6"),
    ("[4] storeGeneratedProblem", "id = FNV-32a(expression); existing -> collision; problemManager.Create -> create"),
    ("funnel log", "requested returned lexer unknown_rules collision answer envelope validator create inserted"),
])
ent("heuristic_generator (server/generator/heuristic2.go)", "pipeline", [
    ("entry", "BuildProblem / BuildProblemRaw / BuildWordSkeletonRaw -> buildRaw"),
    ("planConfig", "buildCtx: conceptFactor, chooseConcepts, sampleConcepts, clampOperandCap, bracketCap"),
    ("buildOne", "chooseAnswer + recursive expand / splitValue: splitAdd, splitSub, splitMul, splitDiv, splitPercent, splitMulFrac, splitDivFrac, splitDivIntByFrac, splitMulIntWithFrac -> realizeLeaf"),
    ("equations", "buildVariableEquation / buildMissingEquation; fallback"),
    ("output", "mathcore.Node AST via Render; exports VERSION, OptionsError"),
])
ent("llm_generator (server/llm_generator)", "pipeline", [
    ("NarrateProblems([]Skeleton)", "openai.GPT5Nano; pairNarrations, firstTopicHint, TopicPromptHint"),
    ("ValidateWordProblem(*Problem)", "openai.GPT5; parseValidatorResponse"),
    ("chatCompletionWithRetry", "isRetryableOpenAIError / isRetryableStatus; withOpenAIErrorCode prefixes openai_code=<code>"),
    ("VERSION", "stamped into problems.generator for WORD problems"),
])
ent("mathcore (leaf kernel, no api/generator imports)", "pipeline", [
    ("AdmitExpression", "stamping.go; ADMISSION_FUNNEL stages [0]-[2.5]; [3] / [3.5] are separate functions"),
    ("ComputeProblemDifficulty", "difficulty.go; + ComputeDifficultyBreakdown, TargetDifficultyRange, Max/MinDiffForBitmap"),
    ("eval", "Eval / EvalTokens / EvalTokensNaiveLTR (requiresPEMDAS); AnswersEquivalent"),
    ("bitmaps", "ValidBitmap / EnumerateValidBitmaps; ProblemType iota + ProblemTypeToFeatures"),
])
ent("MATHCORE_AST (ast.go)", "pipeline", [
    ("what it is", "the expression tree behind every problem. heuristic_generator builds one answer-first, Render flattens it to the ASCII stored in problems.expression, and answer-checking re-parses that ASCII back into this same tree"),
    ("example", "\"(3 + 5) * 4 = ?\" is Equation{ LHS: BinaryExpr{ Op:*, L: Paren{ BinaryExpr{ Op:+, Num 3, Num 5 } }, R: Num 4 }, RHS: Missing }"),
    ("Num", "a number literal held as an exact rational (big.Rat), so 1/3 is never 0.333...; flags remember how to display it, not what it's worth: IsDecimal (12.0 vs 12), IsPercent (25% vs 0.25), IsFraction (3/4 vs 0.75); Raw keeps a spelling the value can't reproduce, like unreduced 6/8"),
    ("Missing", "the '?' blank the student solves for"),
    ("Var", "a variable letter like x; HasCoefficient means it renders glued to its number: 3x, not 3 * x"),
    ("BinaryExpr", "one operator and two operands (L Op R; Op is + - * /); nests to form larger expressions"),
    ("Paren", "parentheses the student must see. Kept as a real node because (3 + 5) * 4 and 3 + 5 * 4 are different problems (PEMDAS)"),
    ("Equation", "top-level LHS = RHS, used for variable and missing-value problems"),
    ("Render", "walks the tree into canonical ASCII: operators spaced (a + b); division printed as the obelus (6 ÷ 2) so it can't be read as the fraction slash (3/8 stays a literal); a leading percent factor prints as \"25% of 80\"; parens auto-inserted anywhere infix precedence would re-parse the tree differently"),
    ("the guarantee", "Parse(Render(tree)) rebuilds the same tree and Eval folds it to the same answer, so problems can live as plain text in the DB yet be re-checked exactly"),
])
ent("processEvents (process_events.go)", "pipeline", [
    ("processEvents", "all-record-only batches -> processRecordOnlyEvents (INSERT events only); else loadGamestateAndSettings + per-event branches"),
    ("logged_in", "record-only (no value)"),
    ("selected_problem", "int ProblemID; validate only; post-loop recordRecentlyShown upsert"),
    ("working_on_problem", "int ms; record-only"),
    ("answered_problem", "string answer; correct -> synthesizes solved_problem + advanceReviewQueue; wrong -> addToReviewQueue [1,3,7]d"),
    ("solved_problem", "int ProblemID; synthesized server-side, never sent by client"),
    ("watching_video", "int ms; record-only"),
    ("done_watching_video", "int VideoID; -> ADAPTIVE_ADJUSTER (work-load rebalance + new reward video)"),
    ("error_playing_video", "string error; UPDATE videos SET disabled=1 + selectVideo reselect"),
    ("set_target_difficulty", "float; validate vs TargetDifficultyRange; new problem selection (write happens in customUpdateSettings)"),
    ("set_target_work_percentage", "float; record-only"),
    ("set_problem_type_bitmap", "uint64; validate bits; new problem selection + generateProblemsBackground (write in customUpdateSettings)"),
    ("set_gamestate_target", "uint32; range-validate [5,40] only (writes happen via the adjuster)"),
    ("bad_problem_system / bad_problem_user", "int ProblemID; parseBadProblemID -> UPDATE problems SET status='reported'"),
])
ent("ADAPTIVE_ADJUSTER (process_events.go)", "pipeline", [
    ("input", "work % over last 900 working_on_problem / watching_video rows vs settings.target_work_percentage (deadband 0.05)"),
    ("too easy", "gamestate.target + 1; at max 40 -> halve target and raise target_difficulty by max(1, 0.05 x diff)"),
    ("too hard", "halve target (floor 5); at floor -> target + 1 and lower target_difficulty; at min difficulty -> no easier"),
    ("bounds", "mathcore.TargetDifficultyRange(bitmap); repair clamp on entry"),
    ("then", "solved reset to 0; selectVideo picks a new reward video into gamestate.video_id"),
    ("audit", "set_gamestate_target, set_target_difficulty events"),
])
ent("deploy/update.sh (manual deploy)", "jobs", [
    ("preflight", "conf.json api_port must match nginx proxy_pass; api_host = https://mikeymath.org; maintenance-flag path cross-check"),
    ("build", "make; build-web stages web/build.next then swaps, so a failed build leaves the live site untouched"),
    ("maintenance", "raises /var/www/mathgame/maintenance.on for the restart; a failed deploy ends on the maintenance page (set -e)"),
    ("sync + restart", "unit files -> /etc/systemd/system, daemon-reload; restart mathgame-api + all timers; nginx -t then reload"),
    ("smoke", "loopback readiness (401 on 127.0.0.1:api_port), then front-door curls; failure re-raises maintenance"),
])
ent("mathgame-compress-events (03:00)", "jobs", [
    ("unit", "flock -n /var/lock/<unit>.lock bin/compress_events; Persistent=true"),
    ("PlanCompress", "resume from compress_events_meta.last_event_id"),
    ("CompressEvents", "run-length sums consecutive same-user summable runs (working_on_problem, watching_video)"),
    ("RunCompress", "tx: UPDATE events SET value=CASE id... + DELETE (maxChunkSize 21845)"),
])
ent("mathgame-check-disabled-videos (03:30)", "jobs", [
    ("unit", "flock; bin/check_disabled_videos --enable"),
    ("probe", "YouTube Data API v3 /videos + oembed fallback per disabled=1 row"),
    ("enable", "playable -> UPDATE videos SET disabled=0"),
])
ent("mathgame-update-statistics (04:00)", "jobs", [
    ("unit", "bin/update_statistics_cache, all users; the one timer without flock, shares 04:00 with trim"),
    ("UpdateStatisticsForUser", "incremental from statistics_cache_meta.last_event_id; fullProgressBackfill when absent"),
    ("writes", "statistics_totals, statistics_monthly, statistics_cache_meta"),
])
ent("mathgame-trim-recently-shown-problems (04:00)", "jobs", [
    ("unit", "flock; bin/trim_recently_shown_problems"),
    ("planRecentlyShownTrim", "DELETE rows past recentlyShownProblemsTrimSize per user"),
])
ent("mathgame-watchdog (*:0/5)", "jobs", [
    ("deploy/watchdog.sh", "journalctl -u mathgame-api over the last hour; 1h alert cooldown per watch"),
    ("WATCHES", "openai 5/hr, openai-narrate-content 5/hr, openai-quota-code 1/hr, openai-quota 1/hr"),
    ("alert", "POST ntfy.sh/$ntfy_topic, Priority: high, bodies cut to 300 bytes"),
])
ent("MANUAL_TOOLS (cmd/)", "jobs", [
    ("recompute_problem_type_bitmap", "restamps bitmap via mathcore.AdmitExpression; run first"),
    ("recompute_problem_difficulty", "restamps difficulty; skips rows already at mathcore.DifficultyVersion"),
    ("cleanup_unused_problems", "pool cull; dry-run by default, -apply deletes; full events scan, off-peak only"),
    ("compare_generators", "read-only heuristic_2.0 vs stored heuristic_1.0 / llm_* report"),
])

# --- Tables ------------------------------------------------------------------------
TW = 340
ent("users", "table", [
    ("auth0_id", "VARCHAR(225)"),
    ("id", "BIGINT UNSIGNED AUTO_INCREMENT, surrogate every table joins on"),
    ("email", "VARCHAR(320)"), ("username", "VARCHAR(128)"),
    ("pin", "VARCHAR(4), sent to client deliberately"),
    ("role", "'student' | 'admin'"),
], TW, idx=["PK (auth0_id)", "UNIQUE (id)"])
ent("settings", "table", [
    ("user_id", "FK -> users.id (logical, 1:1)"),
    ("problem_type_bitmap", "BIGINT UNSIGNED"),
    ("target_difficulty", "DOUBLE"),
    ("target_work_percentage", "INT(3)"),
], TW, idx=["PK (user_id)"])
ent("gamestates", "table", [
    ("user_id", "FK -> users.id (logical, 1:1)"),
    ("problem_id", "FK -> problems.id, currently served"),
    ("video_id", "FK -> videos.id, current reward"),
    ("solved", "INT(5)"), ("target", "INT(5)"),
], TW, idx=["PK (user_id)"])
ent("problems", "table", [
    ("id", "BIGINT UNSIGNED, content hash (not a sequence)"),
    ("problem_type_bitmap", "BIGINT UNSIGNED bitfield"),
    ("expression / answer / explanation", "TEXT"),
    ("symbolic_expression", "VARCHAR(512)"),
    ("difficulty", "FLOAT"),
    ("status", "active | deprecated | reported | incorrect"),
    ("generator", "VARCHAR(64); current: heuristic_2.2, llm_0.8 (older versions ranked by generatorRank)"),
    ("difficulty_version", "VARCHAR(16)"),
    ("created_at", "TIMESTAMP"),
], TW, idx=["PK (id)", "idx_problems_status_diff_bitmap (status, difficulty, problem_type_bitmap)"])
ent("events", "table", [
    ("id", "BIGINT UNSIGNED AUTO_INCREMENT"),
    ("timestamp", "TIMESTAMP"),
    ("user_id", "FK -> users.id (logical)"),
    ("event_type", "VARCHAR(32), 14 types in event_types.go"),
    ("value", "TEXT, may carry problems.id / videos.id"),
], TW, idx=["PK (id)", "idx_events_user_event (user_id, event_type)"])
ent("review_queue", "table", [
    ("user_id", "FK -> users.id"),
    ("problem_id", "FK -> problems.id"),
    ("next_review_at", "TIMESTAMP"),
    ("interval_days", "1 | 3 | 7"),
], TW, idx=["PK (user_id, problem_id)"])
ent("recently_shown_problems", "table", [
    ("user_id", "FK -> users.id"),
    ("problem_id", "FK -> problems.id"),
    ("shown_at", "TIMESTAMP"),
], TW, idx=["PK (user_id, problem_id)", "idx_recently_shown_problems_user_time (user_id, shown_at)"])

ent("videos", "table", [
    ("id", "BIGINT UNSIGNED AUTO_INCREMENT"),
    ("title", "VARCHAR(128)"), ("url", "VARCHAR(256)"),
    ("thumbnailurl", "VARCHAR(256)"),
    ("you_tube_id", "VARCHAR(32) NULL"),
    ("disabled", "TINYINT"),
], TW, idx=["PK (id)", "UNIQUE (you_tube_id)"])
ent("playlists", "table", [
    ("id", "BIGINT UNSIGNED AUTO_INCREMENT"),
    ("you_tube_id", "VARCHAR(64)"),
    ("title", "VARCHAR(512)"), ("thumbnailurl", "VARCHAR(1024)"),
    ("etag", "VARCHAR(128)"),
], TW, idx=["PK (id)", "UNIQUE (you_tube_id)"])
ent("playlist_video", "join", [
    ("playlist_id", "FK -> playlists.id (declared)"),
    ("video_id", "FK -> videos.id (declared)"),
], TW, idx=["PK (playlist_id, video_id)"])
ent("user_playlist", "join", [
    ("user_id", "FK -> users.id (declared)"),
    ("playlist_id", "FK -> playlists.id (declared)"),
], TW, idx=["PK (user_id, playlist_id)"])
ent("user_has_video", "join", [
    ("user_id", "FK -> users.id (declared)"),
    ("video_id", "FK -> videos.id (declared)"),
], TW, idx=["PK (user_id, video_id)"])
ent("statistics_cache_meta", "table", [
    ("user_id", "FK -> users.id"),
    ("last_event_id", "watermark -> events.id"),
], TW, idx=["PK (user_id)"])
ent("statistics_totals", "table", [
    ("user_id", "FK -> users.id"),
    ("total_problems_solved / total_work_minutes / total_video_minutes", "BIGINT"),
], TW, idx=["PK (user_id)"])
ent("statistics_monthly", "table", [
    ("user_id", "FK -> users.id"),
    ("month", "CHAR(7) YYYY-MM"),
    ("totals", "solved / work / video minutes, BIGINT"),
], TW, idx=["PK (user_id, month)"])
ent("compress_events_meta", "table", [
    ("dummy", "TINYINT, singleton row = 1"),
    ("last_event_id", "watermark -> events.id"),
], TW, idx=["PK (dummy)"])
ent("calibration_report", "table", [
    ("id", "TINYINT UNSIGNED, singleton row = 1"),
    ("report", "LONGTEXT"), ("computed_at", "TIMESTAMP"),
], TW, idx=["PK (id)"])
ent("bitmap_matrix_report", "table", [
    ("id", "TINYINT UNSIGNED, singleton row = 1"),
    ("report", "LONGBLOB (gzipped)"), ("computed_at", "TIMESTAMP"),
], TW, idx=["PK (id)"])

COLS = [
    (40,   "CLIENT (React SPA)", ["APP_SHELL (index.js, auth0.js)", "PLAY_VIEW (play.js)",
                                  "SETTINGS_SETUP (settings.js, setup.js, pin.js)",
                                  "PROGRESS_ADMIN (progress.js, admin_*.js)"]),
    (600,  "EDGE + API SERVER (Go / gin)", ["EXTERNAL_SERVICES", "NGINX (deploy/nginx/mikeymath.conf)",
                                            "APISERVER (cmd/apiserver, server/api)",
                                            "YOUTUBE_SYNC (youtube.go, custom_handlers.go)"]),
    (1160, "SELECTION + EVENT PIPELINE", ["selectProblem (generate_problems.go)",
                                          "processEvents (process_events.go)",
                                          "ADAPTIVE_ADJUSTER (process_events.go)"]),
    (1720, "PROBLEM GENERATION", ["generateProblems (generate_problems.go)",
                                  "ADMISSION_FUNNEL",
                                  "heuristic_generator (server/generator/heuristic2.go)",
                                  "llm_generator (server/llm_generator)",
                                  "mathcore (leaf kernel, no api/generator imports)",
                                  "MATHCORE_AST (ast.go)"]),
    (2280, "BACKGROUND JOBS (systemd + manual)", ["deploy/update.sh (manual deploy)",
                                                  "mathgame-compress-events (03:00)",
                                                  "mathgame-check-disabled-videos (03:30)",
                                                  "mathgame-update-statistics (04:00)",
                                                  "mathgame-trim-recently-shown-problems (04:00)",
                                                  "mathgame-watchdog (*:0/5)", "MANUAL_TOOLS (cmd/)"]),
    (2840, "CORE TABLES (MySQL: mathgame)", ["users", "settings", "gamestates", "problems", "events",
                                             "review_queue", "recently_shown_problems"]),
    (3280, "CONTENT + CACHE TABLES", ["videos", "playlists", "playlist_video", "user_playlist",
                                      "user_has_video", "statistics_cache_meta", "statistics_totals",
                                      "statistics_monthly",
                                      "compress_events_meta", "calibration_report", "bitmap_matrix_report"]),
]

HDR = 30
def row_h(key, desc, w):
    n = len(key) + len(desc) + 3
    per_line = w / 6.2
    lines = max(1, int(n / per_line) + (1 if n % per_line else 0))
    return 22 * lines + 4

cells = []
cid = [2]
def nid():
    cid[0] += 1
    return f"c{cid[0]}"

pos = {}
for x, title, names in COLS:
    y = 40
    if title:
        cells.append(f'<mxCell id="{nid()}" value="{escape(title)}" style="text;html=1;fontSize=16;fontStyle=1;align=left;verticalAlign=middle;strokeColor=none;fillColor=none;" vertex="1" parent="1"><mxGeometry x="{x}" y="{y}" width="520" height="30" as="geometry"/></mxCell>')
    y += 50
    for name in names:
        e = E[name]
        hf, st, bf = LAYERS[e["layer"]]
        w = e["w"]
        heights = [row_h(k, d, w) for k, d in e["rows"]]
        idx_heights = [row_h(s, "", w) for s in e["idx"]]
        h = HDR + sum(heights) + (8 + sum(idx_heights) if e["idx"] else 0)
        eid = f"e_{name}"
        pos[name] = (x, y, w, h)
        style = (f"swimlane;html=1;fontStyle=1;fontSize=13;childLayout=stackLayout;horizontal=1;startSize={HDR};"
                 f"horizontalStack=0;resizeParent=1;resizeParentMax=0;resizeLast=0;collapsible=0;marginBottom=0;"
                 f"whiteSpace=wrap;fillColor={hf};strokeColor={st};swimlaneFillColor={bf};rounded=1;arcSize=6;shadow=1;")
        cells.append(f'<mxCell id="{eid}" value="{escape(name)}" style="{style}" vertex="1" parent="1"><mxGeometry x="{x}" y="{y}" width="{w}" height="{h}" as="geometry"/></mxCell>')
        ry = HDR
        for (k, d), rh in zip(e["rows"], heights):
            txt = f"<b>{escape(k)}</b>" + (f"&nbsp; {escape(d)}" if d else "")
            rstyle = (f"text;html=1;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=6;spacingRight=6;"
                      f"whiteSpace=wrap;overflow=hidden;fontSize=11;")
            cells.append(f'<mxCell id="{nid()}" value="{escape(txt)}" style="{rstyle}" vertex="1" parent="{eid}"><mxGeometry y="{ry}" width="{w}" height="{rh}" as="geometry"/></mxCell>')
            ry += rh
        if e["idx"]:
            cells.append(f'<mxCell id="{nid()}" style="line;strokeWidth=1;html=1;fillColor=none;strokeColor={st};" vertex="1" parent="{eid}"><mxGeometry y="{ry}" width="{w}" height="8" as="geometry"/></mxCell>')
            ry += 8
            for s, rh in zip(e["idx"], idx_heights):
                istyle = ("text;html=1;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=6;spacingRight=6;"
                          "whiteSpace=wrap;overflow=hidden;fontSize=11;fontStyle=2;fontColor=#666666;")
                cells.append(f'<mxCell id="{nid()}" value="{escape(s)}" style="{istyle}" vertex="1" parent="{eid}"><mxGeometry y="{ry}" width="{w}" height="{rh}" as="geometry"/></mxCell>')
                ry += rh
        y += h + 36

ncols = len(COLS)
col_of = {}
for i, (_, _, names) in enumerate(COLS):
    for n in names:
        col_of[n] = i
col_left = [c[0] for c in COLS]
col_right = [max(pos[n][0] + pos[n][2] for n in names) for _, _, names in COLS]

def corridor_x(i):
    if i < 0:
        return col_left[0] - 50
    if i >= ncols - 1:
        return col_right[ncols - 1] + 50
    left, right = col_right[i], col_left[i + 1]
    assert right - left >= 40, f"corridor {i} is {right-left}px; widen the column spacing"
    return (left + right) / 2

def col_bands(i):
    boxes = sorted((pos[n][1], pos[n][1] + pos[n][3]) for n in COLS[i][2])
    ys = [boxes[0][0] - 18]
    for (_, b1), (t2, _) in zip(boxes, boxes[1:]):
        ys.append((b1 + t2) / 2)
    ys.append(boxes[-1][1] + 18)
    return ys

_use = {}
def _off(key, step=10):
    k = _use.get(key, 0)
    _use[key] = k + 1
    return ((k % 5) - 2) * step

def _side(p):
    x, y = p
    if x == 0: return "L"
    if x == 1: return "R"
    return "T" if y == 0 else "B"

def _abs(name, p):
    x, y, w, h = pos[name]
    return (x + p[0] * w, y + p[1] * h)

def _clear_between(i, ytop, ybot, skip):
    for n in COLS[i][2]:
        if n in skip:
            continue
        t, b = pos[n][1], pos[n][1] + pos[n][3]
        if t < ybot and b > ytop:
            return False
    return True

def route_edge(f, t, exit_pt, entry_pt):
    si, ti = col_of[f], col_of[t]
    S, T = _abs(f, exit_pt), _abs(t, entry_pt)
    es, en = _side(exit_pt), _side(entry_pt)
    if si == ti:
        if es in "TB" and en in "TB" and _clear_between(si, min(S[1], T[1]), max(S[1], T[1]), {f, t}):
            return []
        cx = corridor_x(si - 1 if es == "L" else si) + _off(("v", si, es))
        return [(cx, S[1]), (cx, T[1])]
    step = 1 if ti > si else -1
    pts = []
    y = S[1]
    if es in "TB":
        y = pos[f][1] + pos[f][3] + 14 if es == "B" else pos[f][1] - 14
        pts.append((S[0], y))
    ci = si if step == 1 else si - 1
    cx = corridor_x(ci) + _off(("v", ci))
    pts.append((cx, y))
    j = si + step
    while j != ti:
        band = min(col_bands(j), key=lambda b: abs(b - T[1])) + _off(("h", j), 6)
        pts.append((cx, band))
        ci = j if step == 1 else j - 1
        cx = corridor_x(ci) + _off(("v", ci))
        pts.append((cx, band))
        j += step
    if en in "TB":
        ty = pos[t][1] - 14 if en == "T" else pos[t][1] + pos[t][3] + 14
        pts.append((cx, ty))
        pts.append((T[0], ty))
    else:
        pts.append((cx, T[1]))
    return pts

A = {"1": "ERmandOne", "01": "ERzeroToOne", "0n": "ERzeroToMany", "1n": "ERoneToMany"}
SHELL = "APP_SHELL (index.js, auth0.js)"
PLAY = "PLAY_VIEW (play.js)"
SETT = "SETTINGS_SETUP (settings.js, setup.js, pin.js)"
ADMN = "PROGRESS_ADMIN (progress.js, admin_*.js)"
EXT = "EXTERNAL_SERVICES"
NGX = "NGINX (deploy/nginx/mikeymath.conf)"
API = "APISERVER (cmd/apiserver, server/api)"
SEL = "selectProblem (generate_problems.go)"
ORCH = "generateProblems (generate_problems.go)"
FUN = "ADMISSION_FUNNEL"
HEU = "heuristic_generator (server/generator/heuristic2.go)"
LLM = "llm_generator (server/llm_generator)"
MC = "mathcore (leaf kernel, no api/generator imports)"
AST = "MATHCORE_AST (ast.go)"
PRC = "processEvents (process_events.go)"
ADJ = "ADAPTIVE_ADJUSTER (process_events.go)"
YTS = "YOUTUBE_SYNC (youtube.go, custom_handlers.go)"
UPD = "deploy/update.sh (manual deploy)"
CEV = "mathgame-compress-events (03:00)"
CDV = "mathgame-check-disabled-videos (03:30)"
UST = "mathgame-update-statistics (04:00)"
TRM = "mathgame-trim-recently-shown-problems (04:00)"
WDG = "mathgame-watchdog (*:0/5)"
MAN = "MANUAL_TOOLS (cmd/)"
edges = [
    (SHELL, EXT, "0n", "1", "loginWithRedirect / JWT", "flow", (1, 0.15), (0, 0.3)),
    (SHELL, NGX, "0n", "1", "pageload, users, events", "flow", (1, 0.5), (0, 0.2)),
    (PLAY, NGX, "0n", "1", "GET /play, POST /events", "flow", (1, 0.3), (0, 0.5)),
    (SETT, NGX, "0n", "1", "settings / playlists / users", "flow", (1, 0.4), (0, 0.7)),
    (ADMN, NGX, "0n", "1", "statistics, /admin/*", "flow", (1, 0.5), (0, 0.9)),
    (NGX, API, "1", "1", "proxy /api/ -> 127.0.0.1:8080", "flow", (0.5, 1), (0.5, 0)),
    (API, EXT, "0n", "1", "JWT verify; mgmt delete; playlist sync", "flow", (1, 0.05), (1, 0.6)),
    (API, SEL, "1", "1", "selectProblem (new user, invalid stored problem, set_* events)", "flow", (1, 0.25), (0, 0.2)),
    (API, PRC, "1", "0n", "POST /events -> processEvents", "flow", (1, 0.35), (0, 0.3)),
    (PRC, ADJ, "1", "1", "done_watching_video", "flow", (0.5, 1), (0.5, 0)),
    (API, YTS, "1", "1", "playlist CRUD; video reselect", "flow", (0.5, 1), (0.5, 0)),
    (YTS, EXT, "0n", "1", "YouTube Data API v3", "flow", (1, 0.3), (1, 0.5)),
    (SEL, ORCH, "1", "0n", "pool low -> background / live generation", "flow", (1, 0.5), (0, 0.2)),
    (ORCH, FUN, "1", "0n", "every candidate", "flow", (0.5, 1), (0.5, 0)),
    (ORCH, HEU, "1", "0n", "BuildProblem / BuildWordSkeletonRaw", "flow", (0, 0.3), (0, 0.3)),
    (ORCH, LLM, "1", "0n", "narrate + validate WORD skeletons", "flow", (1, 0.55), (1, 0.3)),
    (ORCH, MC, "1", "0n", "AdmitExpression; ComputeProblemDifficulty", "flow", (0, 0.65), (0, 0.3)),
    (HEU, AST, "1", "0n", "builds nodes answer-first; Render -> normalized ASCII", "flow", (0, 0.7), (0, 0.4)),
    (MC, AST, "1", "1", "EvalTokens: Parse into the AST, fold with Eval", "flow", (0.5, 1), (0.5, 0)),
    (LLM, EXT, "0n", "1", "OpenAI chat completions", "flow", (0, 0.55), (1, 0.35)),
    (UPD, NGX, "1", "1", "sync conf, nginx -t, reload; restart mathgame-api + timers", "flow", (0, 0.5), (1, 0.85)),
    (CDV, EXT, "0n", "1", "playability probe", "flow", (0, 0.5), (1, 0.75)),
    (WDG, EXT, "0n", "1", "ntfy.sh alerts", "flow", (0, 0.5), (1, 0.9)),
    (SEL, "problems", "0n", "0n", "reads active pool", "data", (1, 0.3), (0, 0.2)),
    (SEL, "review_queue", "0n", "0n", "due reviews", "data", (1, 0.1), (0, 0.3)),
    (SEL, "recently_shown_problems", "0n", "0n", "recency reads", "data", (1, 0.55), (0, 0.2)),
    (FUN, "problems", "1", "0n", "storeGeneratedProblem INSERT", "data", (1, 0.85), (0, 0.45)),
    (PRC, "events", "1", "0n", "INSERT batch", "data", (1, 0.15), (0, 0.4)),
    (PRC, "gamestates", "1", "1", "solved, problem_id, video_id", "data", (1, 0.45), (0, 0.5)),
    (PRC, "videos", "1", "01", "error_playing_video -> disabled=1", "data", (1, 0.3), (0, 0.3)),
    (ADJ, "settings", "1", "1", "step target_difficulty (clamped)", "data", (1, 0.3), (0, 0.7)),
    (ADJ, "gamestates", "1", "1", "target changes, solved reset, new video_id", "data", (1, 0.5), (0, 0.7)),
    (ADJ, "events", "0n", "0n", "work % query (last 900 rows)", "data", (1, 0.7), (0, 0.85)),
    (PRC, "review_queue", "1", "0n", "add / advance [1,3,7]d", "data", (1, 0.6), (0, 0.7)),
    (PRC, "recently_shown_problems", "1", "0n", "upsert on SELECTED_PROBLEM", "data", (1, 0.75), (0, 0.6)),
    (PRC, "problems", "1", "01", "status='reported'", "data", (1, 0.9), (0, 0.75)),
    (API, "users", "0n", "1", "pageload, provision; delete purges + anonymizes", "data", (1, 0.55), (0, 0.4)),
    (API, "user_playlist", "0n", "0n", "customAddPlaylist / customRemovePlaylist", "data", (1, 0.65), (0, 0.4)),
    (API, "calibration_report", "1", "1", "admin read / recompute", "data", (1, 0.8), (0, 0.5)),
    (API, "bitmap_matrix_report", "1", "1", "admin read / recompute", "data", (1, 0.95), (0, 0.5)),
    (YTS, "playlists", "0n", "0n", "upserts playlists / videos / playlist_video", "data", (1, 0.6), (0, 0.4)),
    (YTS, "user_has_video", "1", "0n", "refreshUserHasVideo rebuild; selectVideo reads", "data", (1, 0.8), (0, 0.5)),
    (API, "statistics_totals", "0n", "1", "rollup: totals, monthly, cache_meta", "data", (1, 0.75), (0, 0.4)),
    (CEV, "events", "1", "0n", "rewrite + delete compressed runs", "data", (1, 0.5), (0, 0.55)),
    (CEV, "compress_events_meta", "1", "1", "last_event_id watermark", "data", (1, 0.8), (0, 0.5)),
    (CDV, "videos", "1", "0n", "UPDATE disabled=0", "data", (1, 0.5), (0, 0.5)),
    (UST, "events", "0n", "0n", "reads events after watermark", "data", (1, 0.3), (0, 0.7)),
    (UST, "statistics_totals", "1", "1", "totals + monthly rollup", "data", (1, 0.55), (0, 0.6)),
    (UST, "statistics_cache_meta", "1", "1", "advance last_event_id", "data", (1, 0.8), (0, 0.5)),
    (TRM, "recently_shown_problems", "1", "0n", "cap rows per user", "data", (1, 0.5), (0, 0.85)),
    (MAN, "problems", "0n", "0n", "restamp bitmap / difficulty; cull pool", "data", (1, 0.5), (0, 0.92)),
]
KIND = {
    "flow": "strokeColor=#d79b00;strokeWidth=2;fontColor=#8a5a00;",
    "data": "strokeColor=#9673a6;strokeWidth=1.5;dashed=1;fontColor=#5e3f6e;",
}
_lbl = {}
def label_x(source):
    """Relative label position along the edge, staggered per source entity.

    draw.io's default midpoint is exactly where corridor-sharing edges run in
    parallel, so their labels stack. The first segment - just past the exit
    pin - is unique per edge, because exit pins on one box never coincide.
    """
    k = _lbl.get(source, 0)
    _lbl[source] = k + 1
    return round(-0.9 + (k % 4) * 0.1, 2)

for f, t, fc, tc, lbl, kind, ex, en in edges:
    pin = f"exitX={ex[0]};exitY={ex[1]};exitDx=0;exitDy=0;entryX={en[0]};entryY={en[1]};entryDx=0;entryDy=0;"
    style = (f"edgeStyle=orthogonalEdgeStyle;orthogonalLoop=1;jettySize=auto;html=1;rounded=1;"
             f"startArrow={A[fc]};startFill=0;endArrow={A[tc]};endFill=0;fontSize=10;"
             f"labelBackgroundColor=#ffffff;{pin}{KIND[kind]}")
    wps = route_edge(f, t, ex, en)
    geo = f'<mxGeometry relative="1" x="{label_x(f)}" as="geometry">'
    if wps:
        geo += '<Array as="points">' + "".join(f'<mxPoint x="{round(px)}" y="{round(py)}"/>' for px, py in wps) + '</Array>'
    geo += '</mxGeometry>'
    cells.append(f'<mxCell id="{nid()}" value="{escape(lbl)}" style="{style}" edge="1" parent="1" source="e_{f}" target="e_{t}">{geo}</mxCell>')

lg = [("client", "Client screens (React SPA)"), ("server", "nginx + API server (Go / gin)"),
      ("funnel", "Problem selection funnel"), ("pipeline", "Domain pipelines"),
      ("jobs", "systemd timers / tools"), ("table", "Database table (MySQL)"),
      ("join", "Join table / external service")]
lx = 40
ly = max(p[1] + p[3] for p in pos.values()) + 60
cells.append(f'<mxCell id="{nid()}" value="Legend" style="text;html=1;fontStyle=1;fontSize=13;strokeColor=none;fillColor=none;" vertex="1" parent="1"><mxGeometry x="{lx}" y="{ly}" width="200" height="24" as="geometry"/></mxCell>')
for i, (k, txt) in enumerate(lg):
    hf, st, bf = LAYERS[k]
    cells.append(f'<mxCell id="{nid()}" value="{escape(txt)}" style="rounded=1;html=1;fillColor={hf};strokeColor={st};fontSize=11;align=left;spacingLeft=6;" vertex="1" parent="1"><mxGeometry x="{lx}" y="{ly+30+i*30}" width="220" height="24" as="geometry"/></mxCell>')
for i, (k, txt) in enumerate([("flow", "control flow (solid orange)"), ("data", "reads / writes (dashed purple)")]):
    cells.append(f'<mxCell id="{nid()}" value="{escape(txt)}" style="text;html=1;fontSize=11;align=left;strokeColor=none;fillColor=none;" vertex="1" parent="1"><mxGeometry x="{lx+280}" y="{ly+30+i*30}" width="260" height="24" as="geometry"/></mxCell>')
    cells.append(f'<mxCell id="{nid()}" style="html=1;endArrow=ERzeroToMany;startArrow=ERmandOne;endFill=0;startFill=0;{KIND[k]}" edge="1" parent="1"><mxGeometry relative="1" as="geometry"><mxPoint x="{lx+540}" y="{ly+42+i*30}" as="sourcePoint"/><mxPoint x="{lx+620}" y="{ly+42+i*30}" as="targetPoint"/></mxGeometry></mxCell>')

xml = '<mxGraphModel adaptiveColors="auto" grid="0" page="0"><root><mxCell id="0"/><mxCell id="1" parent="0"/>' + "".join(cells) + '</root></mxGraphModel>'
open(DRAWIO_OUT, "w").write(xml)
print("wrote", DRAWIO_OUT)

# Standalone read-only viewer page (draw.io's "Export as HTML" format): pan/zoom
# in any browser, no account. Loads the viewer script from viewer.diagrams.net.
from xml.sax.saxutils import quoteattr
viewer_cfg = json.dumps({"highlight": "#0000ff", "nav": True, "resize": True,
                         "dark-mode": "auto", "toolbar": "zoom layers tags lightbox",
                         "edit": "_blank", "xml": xml})
viewer_html = f"""<!DOCTYPE html>
<html>
<head>
<title>mathgame_2 architecture</title>
<meta charset="utf-8"/>
</head>
<body><div class="mxgraph" style="max-width:100%;border:1px solid transparent;" data-mxgraph={quoteattr(viewer_cfg)}></div>
<script type="text/javascript" src="https://viewer.diagrams.net/js/viewer-static.min.js"></script>
</body>
</html>
"""
open(VIEWER_OUT, "w").write(viewer_html)
print("wrote", VIEWER_OUT)

encoded = quote(xml, safe='')
c = zlib.compressobj(9, zlib.DEFLATED, -15)
raw = c.compress(encoded.encode('utf-8')) + c.flush()
data = base64.b64encode(raw).decode()
payload = json.dumps({"type": "xml", "compressed": True, "data": data})
url = f"https://app.diagrams.net/?pv=0&grid=0#create={quote(payload, safe='')}"
html = f"""<!DOCTYPE html>
<html><head><meta charset="utf-8">
<style>
body{{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f8f9fa}}
.card{{text-align:center;background:white;border-radius:12px;padding:40px;box-shadow:0 2px 8px rgba(0,0,0,0.1)}}
.card h2{{margin:0 0 8px;color:#1a1a1a}} .card p{{margin:0 0 24px;color:#666}}
.btn{{display:inline-block;padding:14px 32px;background:#4285f4;color:white;text-decoration:none;border-radius:8px;font-size:16px;font-weight:500}}
.btn:hover{{background:#3367d6}}
</style></head><body><div class="card"><h2>Diagram Ready</h2><p>mathgame_2 repo architecture &mdash; click to open in draw.io</p>
<a class="btn" href="{url}" target="_blank" rel="noopener noreferrer">Open in draw.io</a></div></body></html>"""
if os.path.isdir(os.path.dirname(HTML_OUT)):
    open(HTML_OUT, "w").write(html)
    print("wrote", HTML_OUT)
