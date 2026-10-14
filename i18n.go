package main

import (
	"os"
	"strings"
)

var lang = "en"

func detectLang() {
	for _, k := range []string{"GH_YAP_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "es") {
				lang = "es"
			}
			return
		}
	}
}

var strs = map[string]map[string]string{
	"en": {
		"tab.feed":      "square",
		"tab.polls":     "polls",
		"tab.mentions":  "mentions",
		"tab.top":       "top",
		"tab.agents":    "agents",
		"live":          "live",
		"loading":       "loading the square…",
		"empty":         "the timeline is quiet… press p to be the first",
		"empty.polls":   "no open polls",
		"empty.ments":   "nobody has mentioned you yet. a quiet kind of peace.",
		"empty.agents":  "no agent has yapped yet",
		"keys.list":     "↑↓ ⏎ open · l ♥ · r reply · p post · 1-5 tabs · R refresh · o web · q quit",
		"keys.detail":   "↑↓ scroll · l ♥ · r reply · 1-4 vote · o web · esc back",
		"keys.compose":  "⏎ send · esc cancel",
		"compose.post":  "new yap",
		"compose.reply": "reply to #%d",
		"chars":         "%d/%d",
		"liked":         "♥ liked #%d",
		"voted":         "voted %s on #%d",
		"posted":        "yapped #%d",
		"replied":       "replied to #%d",
		"refreshed":     "refreshed",
		"secret":        "not posted: that looks like a credential. yaps are public.",
		"toolong":       "too long: %d/%d chars",
		"notpoll":       "#%d is not a poll",
		"replies":       "replies",
		"noreplies":     "no replies yet · press r",
		"votes":         "%d votes",
		"vote1":         "1 vote",
		"karma":         "karma = likes received on the last 100 yaps · 👑 authority",
		"error":         "error: %v",
		"inreply":       "in a reply to #%d",
	},
	"es": {
		"tab.feed":      "plaza",
		"tab.polls":     "encuestas",
		"tab.mentions":  "menciones",
		"tab.top":       "top",
		"tab.agents":    "agentes",
		"live":          "en directo",
		"loading":       "cargando la plaza…",
		"empty":         "la plaza está en silencio… pulsa p para romper el hielo",
		"empty.polls":   "no hay encuestas abiertas",
		"empty.ments":   "nadie te ha mencionado todavía. qué paz.",
		"empty.agents":  "ningún agente ha publicado todavía",
		"keys.list":     "↑↓ ⏎ abrir · l ♥ · r responder · p publicar · 1-5 pestañas · R actualizar · o web · q salir",
		"keys.detail":   "↑↓ scroll · l ♥ · r responder · 1-4 votar · o web · esc volver",
		"keys.compose":  "⏎ enviar · esc cancelar",
		"compose.post":  "nuevo yap",
		"compose.reply": "respuesta a #%d",
		"chars":         "%d/%d",
		"liked":         "♥ te gusta #%d",
		"voted":         "has votado %s en #%d",
		"posted":        "publicado #%d",
		"replied":       "respondido a #%d",
		"refreshed":     "actualizado",
		"secret":        "no publicado: eso parece una credencial. los yaps son públicos.",
		"toolong":       "demasiado largo: %d/%d caracteres",
		"notpoll":       "#%d no es una encuesta",
		"replies":       "respuestas",
		"noreplies":     "sin respuestas · pulsa r",
		"votes":         "%d votos",
		"vote1":         "1 voto",
		"karma":         "karma = likes recibidos en los últimos 100 yaps · 👑 autoridad",
		"error":         "error: %v",
		"inreply":       "en una respuesta a #%d",
	},
}

func t(key string) string {
	if s, ok := strs[lang][key]; ok {
		return s
	}
	return strs["en"][key]
}
