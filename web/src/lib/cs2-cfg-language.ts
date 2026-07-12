// CS2 .cfg syntax support for CodeMirror 6: a lightweight StreamLanguage that
// highlights comments, the leading command/cvar token, quoted strings and
// numbers, plus an autocomplete source of common CS2 server-side cvars/commands.
import { StreamLanguage, LanguageSupport } from "@codemirror/language"
import type { StreamParser } from "@codemirror/language"
import { autocompletion } from "@codemirror/autocomplete"
import type { CompletionContext, CompletionResult } from "@codemirror/autocomplete"

// Commands that read as keywords wherever they appear on a line.
const KEYWORDS = new Set(["bind", "unbind", "exec", "alias", "echo", "give", "ent_fire"])

// Common CS2 server-side cvars / commands offered in autocomplete. Kept in sync
// with the set called out in the feature spec.
export const CS2_CVARS: string[] = [
  "sv_cheats",
  "mp_roundtime",
  "mp_roundtime_defuse",
  "mp_roundtime_hostage",
  "mp_freezetime",
  "mp_warmuptime",
  "mp_warmup_pausetimer",
  "mp_restartgame",
  "mp_buy_anywhere",
  "mp_buytime",
  "mp_startmoney",
  "mp_maxmoney",
  "bot_add",
  "bot_add_ct",
  "bot_add_t",
  "bot_kick",
  "bot_quota",
  "bot_difficulty",
  "sv_infinite_ammo",
  "sv_grenade_trajectory_prac_pipreview",
  "sv_grenade_trajectory_prac_trailtime",
  "sv_showimpacts",
  "sv_showimpacts_time",
  "ammo_grenade_limit_total",
  "mp_ct_default_grenades",
  "mp_t_default_grenades",
  "give",
  "ent_fire",
  "sv_gravity",
  "sv_airaccelerate",
  "sv_friction",
  "sv_wateraccelerate",
  "mp_halftime",
  "mp_maxrounds",
  "mp_overtime_enable",
  "mp_overtime_maxrounds",
  "mp_overtime_startmoney",
  "exec",
  "alias",
  "bind",
  "unbind",
  "echo",
  "host_workshop_map",
  "changelevel",
  "map",
  "kick",
  "kickid",
  "banid",
  "addip",
  "removeip",
  "writeid",
  "writeip",
  "status",
  "users",
  "say",
  "rcon_password",
  "sv_password",
  "sv_alltalk",
  "sv_deadtalk",
  "sv_full_alltalk",
  "mp_limitteams",
  "mp_autoteambalance",
  "mp_friendlyfire",
  "ff_damage_reduction_bullets",
  "mp_teammates_are_enemies",
]

// State tracks whether we are past the first token on the current line, so the
// leading command/cvar can be highlighted differently from its arguments.
interface CfgState {
  sawToken: boolean
}

const cfgParser: StreamParser<CfgState> = {
  startState: () => ({ sawToken: false }),
  token(stream, state) {
    // Whole-line comment or inline comment.
    if (stream.match("//")) {
      stream.skipToEnd()
      return "comment"
    }
    // Quoted string.
    if (stream.match(/"(?:[^"\\]|\\.)*"?/)) {
      return "string"
    }
    // Numbers (including decimals and negatives).
    if (stream.match(/-?\d+(?:\.\d+)?/)) {
      return "number"
    }
    // Bare token (command / cvar / argument).
    const m = stream.match(/[^\s"]+/) as RegExpMatchArray | null
    if (m) {
      const word = m[0]
      if (!state.sawToken) {
        state.sawToken = true
        if (KEYWORDS.has(word.toLowerCase())) return "keyword"
        return "variableName" // the leading cvar/command name
      }
      if (KEYWORDS.has(word.toLowerCase())) return "keyword"
      return "atom" // subsequent arguments
    }
    // Whitespace / line breaks.
    if (stream.eol()) {
      state.sawToken = false
    }
    stream.next()
    return null
  },
  blankLine(state) {
    state.sawToken = false
  },
}

export const cs2CfgLanguage = StreamLanguage.define(cfgParser)

// cvarCompletions offers the common CS2 cvars/commands while typing a word.
function cvarCompletions(context: CompletionContext): CompletionResult | null {
  const word = context.matchBefore(/[\w+-]*/)
  if (!word) return null
  if (word.from === word.to && !context.explicit) return null
  return {
    from: word.from,
    options: CS2_CVARS.map((c) => ({
      label: c,
      type: KEYWORDS.has(c) ? "keyword" : "variable",
    })),
    validFor: /^[\w+-]*$/,
  }
}

// cs2Cfg bundles the language with its autocomplete source for the editor.
export function cs2Cfg(): LanguageSupport {
  return new LanguageSupport(cs2CfgLanguage, [autocompletion({ override: [cvarCompletions] })])
}
