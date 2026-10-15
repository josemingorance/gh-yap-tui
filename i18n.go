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
		"keys.list":     "↑↓ ⏎ open · l ♥ · r reply · m message · p post · / search · 1-6 tabs · q quit",
		"keys.detail":   "↑↓ scroll · l ♥ · r reply · m message · 1-4 vote · o web · esc back",
		"keys.search":   "type to filter · ⏎ done · esc clear",
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
		"karma":         "karma = likes received on the last 100 yaps · 👑 authority · badges: gh yap @user",
		"error":         "error: %v",
		"inreply":       "in a reply to #%d",
		"dm.usage":      "usage:\n  gh yap dm list                    your conversations\n  gh yap dm read @user              read your messages with @user\n  gh yap dm send @user \"message\"    send an encrypted message (--dry-run to check first)\n  gh yap dm keys @user              show the SSH keys a message to @user is encrypted to",
		"dm.notice":     "🔐 gh-yap private messages · read this once\n\n  ✓ the content is encrypted to the SSH keys on your and your contact's GitHub profiles: only you two can read it\n  ✗ WHO talks to WHOM, WHEN and HOW MUCH is public (it lives in a public repo)\n  ✗ it's as safe as your SSH private key. if that key is leaked, so are its messages (no forward secrecy)\n    tip: don't reuse the key you use for work servers\n  ✗ anyone with SSH keys on GitHub can message you\n  ✗ AI agents can't use DMs, by design\n",
		"dm.confirm":    "type yes to continue: ",
		"dm.declined":   "ok, nothing was sent or read",
		"dm.agent":      "private messages are off-limits to AI agents",
		"dm.tty":        "private messages only work in an interactive terminal",
		"dm.badflag":    "unknown option %s, nothing was sent",
		"dm.baduser":    "'%s' doesn't look like a GitHub username",
		"dm.none":       "no conversations yet. start one with: gh yap dm send @user \"hi\"",
		"dm.count":      "%d messages",
		"dm.keysfor":    "a message to @%s is encrypted to these keys from their GitHub profile:",
		"dm.nokeys":     "@%s has no ed25519 or RSA SSH keys on GitHub, so nobody can encrypt to them (github.com/settings/keys)",
		"dm.nokey":      "no SSH private key found (~/.ssh/id_ed25519 or ~/.ssh/id_rsa). set GH_YAP_SSH_KEY to the key that matches your GitHub profile",
		"dm.passphrase": "passphrase for your SSH key: ",
		"dm.dryrun":     "dry run · nothing was sent. it would be encrypted to %d key(s) of @%s and to yours:\n    %s",
		"dm.sent":       "🔐 encrypted and sent to @%s",
		"dm.empty":      "no messages with @%s yet",
		"dm.header":     "🔐 conversation with @%s · decrypted with %s",
		"dm.cantread":   "🔒 can't decrypt this one with your key",
		"tab.dms":       "private",
		"keys.dm":       "↑↓ scroll · r write · R refresh · esc back",
		"dm.tabhint":    "encrypted with your GitHub SSH keys · who talks to whom is public · m on any yap to message its author",
		"dm.tuiconfirm": "press y to accept, any other key to go back",
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
		"keys.list":     "↑↓ ⏎ abrir · l ♥ · r responder · m mensaje · p publicar · / buscar · 1-6 pestañas · q salir",
		"keys.detail":   "↑↓ scroll · l ♥ · r responder · m mensaje · 1-4 votar · o web · esc volver",
		"keys.search":   "escribe para filtrar · ⏎ listo · esc borrar",
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
		"karma":         "karma = likes recibidos en los últimos 100 yaps · 👑 autoridad · insignias: gh yap @usuario",
		"error":         "error: %v",
		"inreply":       "en una respuesta a #%d",
		"dm.usage":      "uso:\n  gh yap dm list                     tus conversaciones\n  gh yap dm read @usuario            leer tus mensajes con @usuario\n  gh yap dm send @usuario \"mensaje\"  enviar un mensaje cifrado (--dry-run para comprobarlo antes)\n  gh yap dm keys @usuario            ver con qué claves SSH se cifra un mensaje a @usuario",
		"dm.notice":     "🔐 mensajes privados de gh-yap · léelo una vez\n\n  ✓ el contenido se cifra con las claves SSH de tu perfil de GitHub y del de tu contacto: solo vosotros dos podéis leerlo\n  ✗ QUIÉN habla con QUIÉN, CUÁNDO y CUÁNTO es público (vive en un repo público)\n  ✗ es tan seguro como tu clave SSH privada. si se filtra esa clave, se filtran sus mensajes (no hay secreto hacia adelante)\n    consejo: no reutilices la clave de los servidores del trabajo\n  ✗ cualquiera con claves SSH en GitHub puede escribirte\n  ✗ los agentes de IA no pueden usar los mensajes privados, a propósito\n",
		"dm.confirm":    "escribe sí para continuar: ",
		"dm.declined":   "vale, no se ha enviado ni leído nada",
		"dm.agent":      "los mensajes privados están vetados a los agentes de IA",
		"dm.tty":        "los mensajes privados solo funcionan en una terminal interactiva",
		"dm.badflag":    "opción desconocida %s, no se ha enviado nada",
		"dm.baduser":    "'%s' no parece un usuario de GitHub",
		"dm.none":       "todavía no tienes conversaciones. empieza una con: gh yap dm send @usuario \"hola\"",
		"dm.count":      "%d mensajes",
		"dm.keysfor":    "un mensaje a @%s se cifra con estas claves de su perfil de GitHub:",
		"dm.nokeys":     "@%s no tiene claves SSH ed25519 ni RSA en GitHub, así que nadie puede cifrarle mensajes (github.com/settings/keys)",
		"dm.nokey":      "no encuentro tu clave SSH privada (~/.ssh/id_ed25519 o ~/.ssh/id_rsa). indica con GH_YAP_SSH_KEY la que corresponde a tu perfil de GitHub",
		"dm.passphrase": "contraseña de tu clave SSH: ",
		"dm.dryrun":     "simulación · no se ha enviado nada. se cifraría con %d clave(s) de @%s y con las tuyas:\n    %s",
		"dm.sent":       "🔐 cifrado y enviado a @%s",
		"dm.empty":      "todavía no hay mensajes con @%s",
		"dm.header":     "🔐 conversación con @%s · descifrada con %s",
		"dm.cantread":   "🔒 este no se puede descifrar con tu clave",
		"tab.dms":       "privados",
		"keys.dm":       "↑↓ scroll · r escribir · R actualizar · esc volver",
		"dm.tabhint":    "cifrados con vuestras claves SSH de GitHub · quién habla con quién es público · m sobre un yap para escribir a su autor",
		"dm.tuiconfirm": "pulsa s para aceptar, cualquier otra tecla para volver",
	},
}

func t(key string) string {
	if s, ok := strs[lang][key]; ok {
		return s
	}
	return strs["en"][key]
}
