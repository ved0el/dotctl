#!/bin/bash
# =============================================================================
# Claude Code 3-line statusline — vibe-coding edition
# =============================================================================
# Line 1  WHERE:    📁 cwd  🔀 branch[✱] [↑↓]  +adds −dels  🌳 worktree
# Line 2  ENGINE:   🤖 model  🎚️ effort  🎯 cache-hit%  🧠 used%
# Line 3  STATUS:   💵 cost  ⏱️ duration  🚦 5h X% (2pm)  🚦 7d Y% (Mon 9am)  👤 agent
# =============================================================================

input=$(cat)
J() { jq -r "$1 // empty" 2>/dev/null <<<"$input"; }
SETTINGS="$HOME/.claude/settings.json"

# Optional: capture last render for debugging (set DEBUG_STATUSLINE=1 in env)
if [ "${DEBUG_STATUSLINE:-0}" = "1" ]; then
	{
		echo "--- INPUT @ $(date '+%H:%M:%S') ---"
		printf '%s\n' "$input"
	} >/tmp/cc-statusline-debug.log
fi

# --- Extract (official schema) -----------------------------------------------
MODEL=$(J '.model.display_name')
DIR=$(J '.workspace.current_dir // .cwd')
AGENT=$(J '.agent.name')
WORKTREE=$(J '.worktree.name // .workspace.git_worktree')

EFFORT=$(J '.effort.level')
[ -z "$EFFORT" ] && [ -f "$SETTINGS" ] &&
	EFFORT=$(jq -r '.effortLevel // empty' "$SETTINGS" 2>/dev/null)

COST=$(J '.cost.total_cost_usd // 0')
DUR_MS=$(J '.cost.total_duration_ms // 0')
PCT_USED=$(J '.context_window.used_percentage // 0' | cut -d. -f1)

# Cache hit rate — read token usage from the transcript JSONL, since the
# statusline JSON payload does not expose per-token cache counters.
# We sum cache_read_input_tokens, input_tokens, and cache_creation_input_tokens
# across all assistant turns that have a .message.usage block, then compute:
#   hit% = cache_read * 100 / (cache_read + input_tokens + cache_creation)
# (cache_creation is uncached input, so it belongs in the denominator.)
# Performance: cap at the last 2000 lines so large transcripts stay fast.
CACHE_HIT=""
TRANSCRIPT=$(J '.transcript_path')
if [ -n "$TRANSCRIPT" ] && [ -f "$TRANSCRIPT" ]; then
	read -r _cr _inp _cc < <(
		tail -n 2000 "$TRANSCRIPT" \
		| jq -s -r '
			[ .[]
			  | select(.type == "assistant" and .message.usage != null)
			  | .message.usage
			] | [
			    (map(.cache_read_input_tokens  // 0) | add // 0),
			    (map(.input_tokens             // 0) | add // 0),
			    (map(.cache_creation_input_tokens // 0) | add // 0)
			  ] | @tsv
		' 2>/dev/null | tr -d '\r'
	)
	_denom=$(( ${_cr:-0} + ${_inp:-0} + ${_cc:-0} ))
	if [ "${_cr:-0}" -gt 0 ] && [ "$_denom" -gt 0 ]; then
		CACHE_HIT=$(( _cr * 100 / _denom ))
	fi
fi

RL5_PCT=$(J '.rate_limits.five_hour.used_percentage')
RL5_RESET=$(J '.rate_limits.five_hour.resets_at')
RL7_PCT=$(J '.rate_limits.seven_day.used_percentage')
RL7_RESET=$(J '.rate_limits.seven_day.resets_at')

# --- Colors ------------------------------------------------------------------
C='\033[36m' M='\033[35m' G='\033[32m' Y='\033[33m' R='\033[31m'
K='\033[90m' W='\033[1m' D='\033[2m' X='\033[0m'
BR='\033[1;31m' BY='\033[1;33m' BM='\033[1;35m' BC='\033[1;36m'

# --- Icons (emoji set — swap any of these to taste) --------------------------
ICON_DIR="📁"
ICON_GIT="🔀"
ICON_MODEL="🤖"
ICON_EFFORT="🎚️"
ICON_CACHE="🎯"
ICON_CTX="🧠"
ICON_COST="💵"
ICON_TIME="⏱️"
ICON_LIMIT="🚦"
ICON_AGENT="👤"
ICON_TREE="🌳"

# --- Helpers -----------------------------------------------------------------
short_path() {
	local p="${1/#$HOME/~}"
	local s="${p//[^\/]/}"
	if [ "${#s}" -gt 3 ]; then
		p="…/$(awk -F/ '{print $(NF-2)"/"$(NF-1)"/"$NF}' <<<"$p")"
	fi
	echo "$p"
}

# 3-tier threshold color: returns RED if val>=hi, YELLOW if >=mid, else GREEN
pct_color() {
	if [ "$1" -ge "$2" ]; then
		echo "$R"
	elif [ "$1" -ge "$3" ]; then
		echo "$Y"
	else echo "$G"; fi
}

# Format an epoch into a date string. Tries BSD (date -r) then GNU (date -d @epoch).
fmt_epoch() {
	local epoch="$1" fmt="$2"
	date -r "$epoch" "$fmt" 2>/dev/null \
		|| date -d "@$epoch" "$fmt" 2>/dev/null
}

fmt_reset() {
	local ts="$1"
	[ -z "$ts" ] && return
	local epoch=""
	if [[ "$ts" =~ ^[0-9]{10}$ ]]; then
		epoch="$ts"
	elif [[ "$ts" =~ ^[0-9]{13}$ ]]; then
		epoch=$((ts / 1000))
	else
		epoch=$(date -j -f "%Y-%m-%dT%H:%M:%SZ" "$ts" +%s 2>/dev/null)
		[ -z "$epoch" ] && epoch=$(date -j -f "%Y-%m-%dT%H:%M:%S" "${ts%Z}" +%s 2>/dev/null)
		[ -z "$epoch" ] && epoch=$(date -d "$ts" +%s 2>/dev/null)
	fi
	[ -z "$epoch" ] && return

	local today that_day
	today=$(date +%Y%m%d)
	that_day=$(fmt_epoch "$epoch" +%Y%m%d)
	if [ "$today" = "$that_day" ]; then
		fmt_epoch "$epoch" "+%-I%p" | tr A-Z a-z
	else
		fmt_epoch "$epoch" "+%a %-I%p" | sed -E 's/AM$/am/; s/PM$/pm/'
	fi
}

# --- Context color (used %) -------------------------------------------------
CTX_C=$(pct_color "$PCT_USED" 70 40)

# --- Effort color ------------------------------------------------------------
case "$EFFORT" in
high) EFFORT_C="$R" ;;
medium) EFFORT_C="$Y" ;;
low) EFFORT_C="$G" ;;
*) EFFORT_C="$D" ;;
esac

# --- Git ---------------------------------------------------------------------
GIT_PART=""
FILES_CHANGED=""
ADDS=""
DELS=""
DIR_REAL="${DIR/#\~/$HOME}"
if git -C "$DIR_REAL" rev-parse --git-dir &>/dev/null; then
	BRANCH=$(git -C "$DIR_REAL" branch --show-current 2>/dev/null)
	[ -z "$BRANCH" ] && BRANCH=$(git -C "$DIR_REAL" rev-parse --short HEAD 2>/dev/null)
	DIRTY=""
	[ -n "$(git -C "$DIR_REAL" status --porcelain 2>/dev/null)" ] && DIRTY="${R}✱${X}"
	AB_RAW=$(git -C "$DIR_REAL" rev-list --left-right --count '@{u}...HEAD' 2>/dev/null)
	AB=""
	if [ -n "$AB_RAW" ]; then
		BEHIND="${AB_RAW%%[[:space:]]*}"
		AHEAD="${AB_RAW##*[[:space:]]}"
		[ "$AHEAD" != "0" ] && AB+=" ${Y}↑${AHEAD}${X}"
		[ "$BEHIND" != "0" ] && AB+=" ${Y}↓${BEHIND}${X}"
	fi
	GIT_PART=" ${M}${ICON_GIT} ${BRANCH}${X}${DIRTY}${AB}"

	# Live diff stats vs HEAD (staged + unstaged tracked changes)
	SHORTSTAT=$(git -C "$DIR_REAL" diff HEAD --shortstat 2>/dev/null)
	FILES_CHANGED=$(echo "$SHORTSTAT" | grep -oE '[0-9]+ files? changed' | grep -oE '^[0-9]+')
	ADDS=$(echo "$SHORTSTAT" | grep -oE '[0-9]+ insertions?\(\+\)' | grep -oE '^[0-9]+')
	DELS=$(echo "$SHORTSTAT" | grep -oE '[0-9]+ deletions?\(-\)' | grep -oE '^[0-9]+')
fi

# --- Edits (git diff --shortstat: Nf +adds −dels) ----------------------------
# Skip the segment entirely if file count can't be determined.
EDITS=""
if [ -n "$FILES_CHANGED" ] && [ "$FILES_CHANGED" -gt 0 ] 2>/dev/null; then
	EDITS="${K}${FILES_CHANGED}f${X}"
	[ -n "$ADDS" ] && [ "$ADDS" -gt 0 ] && EDITS+=" ${G}+${ADDS}${X}"
	[ -n "$DELS" ] && [ "$DELS" -gt 0 ] && EDITS+=" ${R}−${DELS}${X}"
fi

# --- Duration ----------------------------------------------------------------
DUR=""
if [ "$DUR_MS" -gt 0 ]; then
	MINS=$((DUR_MS / 60000))
	SECS=$(((DUR_MS % 60000) / 1000))
	if [ "$MINS" -ge 60 ]; then
		DUR="$((MINS / 60))h$((MINS % 60))m"
	elif [ "$MINS" -gt 0 ]; then
		DUR="${MINS}m"
	else
		DUR="${SECS}s"
	fi
fi

# --- Cost --------------------------------------------------------------------
COST_FMT=$(printf '%.2f' "$COST")
COST_INT=$(printf '%.0f' "$COST")
if [ "$COST_INT" -ge 5 ]; then COST_C="$BR"; else COST_C="$BY"; fi

# --- Rate-limit segment ------------------------------------------------------
fmt_rl() {
	local label="$1" pct="$2" reset_ts="$3"
	[ -z "$pct" ] && return
	pct=$(printf '%.0f' "$pct")
	local pct_c
	pct_c=$(pct_color "$pct" 80 50)
	local out="${ICON_LIMIT} ${pct_c}${label} ${pct}%${X}"
	local reset_str
	reset_str=$(fmt_reset "$reset_ts")
	[ -n "$reset_str" ] && out+=" ${K}(${reset_str})${X}"
	echo "$out"
}

# --- LINE 1: WHERE -----------------------------------------------------------
DIR_SHORT=$(short_path "$DIR")
LINE1="${C}${ICON_DIR} ${DIR_SHORT}${X}${GIT_PART}"
[ -n "$EDITS" ] && LINE1+=" ${EDITS}"
[ -n "$WORKTREE" ] && LINE1+=" ${BM}${ICON_TREE} ${WORKTREE}${X}"

# --- LINE 2: ENGINE (model · effort · cache · ctx) ---------------------------
LINE2="${BC}${ICON_MODEL} ${MODEL}${X}"
[ -n "$EFFORT" ] && LINE2+=" ${ICON_EFFORT} ${EFFORT_C}${EFFORT}${X}"
if [ -n "$CACHE_HIT" ]; then
	# High hit rate is good — invert the usual thresholds so green = ≥80%.
	if [ "$CACHE_HIT" -ge 80 ]; then
		CACHE_C="$G"
	elif [ "$CACHE_HIT" -ge 50 ]; then
		CACHE_C="$Y"
	else
		CACHE_C="$R"
	fi
	LINE2+=" ${ICON_CACHE} ${CACHE_C}${CACHE_HIT}%${X}"
fi
LINE2+=" ${ICON_CTX} ${CTX_C}${W}${PCT_USED}%${X}"

# --- LINE 3: STATUS (cost · duration · rate limits · agent) ------------------
LINE3=""
[ "$COST_FMT" != "0.00" ] && LINE3+="${COST_C}${ICON_COST} \$${COST_FMT}${X}"
[ -n "$DUR" ] && LINE3+="${LINE3:+ }${K}${ICON_TIME} ${DUR}${X}"
RL5=$(fmt_rl "5h" "$RL5_PCT" "$RL5_RESET")
RL7=$(fmt_rl "7d" "$RL7_PCT" "$RL7_RESET")
[ -n "$RL5" ] && LINE3+="${LINE3:+ }$RL5"
[ -n "$RL7" ] && LINE3+="${LINE3:+ }$RL7"
[ -n "$AGENT" ] && LINE3+="${LINE3:+ }${BY}${ICON_AGENT} ${AGENT}${X}"

# --- Emit --------------------------------------------------------------------
echo -e "$LINE1"
echo -e "$LINE2"
[ -n "$LINE3" ] && echo -e "$LINE3"
exit 0
