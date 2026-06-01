#!/usr/bin/env zsh
# tools profile — shell integration for the mise-managed CLI tools.
#
# Sourced by ~/.zshrc's conf.d loop (after mise activates, so tool shims are on
# PATH). This is shell config — aliases, env, fzf/zoxide init — NOT package
# lifecycle (install lives in the mise conf.d manifest, profiles/tools/config/mise).
#
# Each block self-gates on `command -v <tool>` so a tool absent from PATH (tools
# profile not applied, or before `mise install` ran) silently no-ops.

# ── bat ──────────────────────────────────────────────────────────────────────
# Use bat as the man pager unless the user already chose one.
if command -v bat &>/dev/null && [[ -z "${MANPAGER:-}" ]]; then
  export MANPAGER="sh -c 'col -bx | bat -l man -p'"
fi

# ── eza ──────────────────────────────────────────────────────────────────────
if command -v eza &>/dev/null; then
  typeset _e="eza --group-directories-first --icons=auto"
  alias ls="$_e"
  alias la="$_e -a"
  alias ll="$_e -l --git --time-style=relative"
  alias lla="$_e -la --git --time-style=relative"
  alias lt="$_e --tree"
  alias lt2="$_e --tree --level=2"
  alias lt3="$_e --tree --level=3"
  alias lta="$_e --tree -a"
  alias lm="$_e -l --sort=modified --reverse --time-style=relative"
  alias lz="$_e -l --sort=size --reverse"
  unset _e
fi

# ── fd ───────────────────────────────────────────────────────────────────────
# --follow: cross symlinks. --hidden: include dotfiles (exclusions live in
# config/fd/ignore so they stay version-controlled out of this env var).
command -v fd &>/dev/null && export FD_OPTIONS="--follow --hidden"

# ── ripgrep ──────────────────────────────────────────────────────────────────
command -v rg &>/dev/null &&
  export RIPGREP_CONFIG_PATH="${XDG_CONFIG_HOME:-$HOME/.config}/ripgrep/ripgreprc"

# ── fzf (before zoxide so _ZO_FZF_OPTS can inherit FZF defaults) ──────────────
if command -v fzf &>/dev/null; then
  export FZF_DEFAULT_COMMAND="fd --type f"
  export FZF_DEFAULT_OPTS="--height 75% --multi --reverse --margin=0,1 \
    --bind ctrl-f:page-down,ctrl-b:page-up,ctrl-/:toggle-preview \
    --bind pgdn:preview-page-down,pgup:preview-page-up \
    --marker='✚' --pointer='▶' --prompt='❯ ' --no-separator --scrollbar='█' \
    --color bg+:#262626,fg+:#dadada,hl:#f09479,hl+:#f09479 \
    --color border:#303030,info:#cfcfb0,header:#80a0ff,spinner:#36c692 \
    --color prompt:#87afff,pointer:#ff5189,marker:#f09479"
  export FZF_CTRL_R_OPTS="--no-preview"
  export FZF_CTRL_T_COMMAND="rg --files --hidden --follow --glob '!.git/*'"
  export FZF_CTRL_T_OPTS="--preview 'bat --line-range :100 {}'"
  export FZF_ALT_C_COMMAND="fd --type d"
  if command -v eza &>/dev/null; then
    export FZF_ALT_C_OPTS="--preview 'eza --tree --level 2 --group-directories-first {}'"
  fi

  # Keybindings AFTER env vars so they inherit them.
  eval "$(fzf --zsh)"

  # macOS: Option+C sends ç instead of the ESC-c sequence fzf expects.
  [[ "$(uname)" == "Darwin" ]] && bindkey 'ç' fzf-cd-widget 2>/dev/null
fi

# ── zoxide ───────────────────────────────────────────────────────────────────
if command -v zoxide &>/dev/null; then
  # Suppress doctor warning: sheldon's zsh-defer loads plugins after our init,
  # which trips zoxide's order check. Functionality is unaffected.
  export _ZO_DOCTOR=0
  eval "$(zoxide init zsh)"
  alias cd="z"
  alias cdi="zi"
  if command -v eza &>/dev/null; then
    export _ZO_FZF_OPTS="--preview 'eza -al --tree --level 1 --group-directories-first \
      --header --no-user --no-time --no-filesize --no-permissions {2..}' \
      --preview-window right,50% --height 35% --reverse --ansi --with-nth 2.."
  fi
fi

# ── gh (GitHub CLI) — completion ──────────────────────────────────────────────
if command -v gh &>/dev/null; then
  eval "$(gh completion -s zsh)"
fi

# ── dotctl (dotfile manager) — completion ──────────────────────────────────────
if command -v dotctl &>/dev/null; then
  eval "$(dotctl completion zsh)"
fi
# Tools needing no shell integration (pure binaries): jq, yq, btop, tree.
# delta is configured via git (set `core.pager = delta` in your ~/.gitconfig).
