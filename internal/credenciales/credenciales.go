// Package credenciales averigua con qué va a autenticarse el SDK de Anthropic.
//
// Existe porque el SDK resuelve credenciales por orden de precedencia y falla
// de formas confusas cuando hay dos configuradas a la vez. El caso más cruel:
// una ANTHROPIC_API_KEY *definida pero vacía* gana sobre la federación de
// identidades y la deja inservible, sin decir nada. Detectarlo aquí, en el
// comando doctor, cuesta un segundo; descubrirlo en producción cuesta una tarde.
package credenciales

import (
	"fmt"
	"os"
	"strings"
)

// Los cuatro que exige la federación de identidades. El token puede venir de un
// archivo (lo habitual en GCP/AWS/Azure) o de una variable (GitHub Actions).
var varsFederacion = []string{
	"ANTHROPIC_FEDERATION_RULE_ID",
	"ANTHROPIC_ORGANIZATION_ID",
	"ANTHROPIC_SERVICE_ACCOUNT_ID",
}

type Metodo int

const (
	Ninguno Metodo = iota
	APIKey
	AuthToken
	PerfilOAuth
	Federacion
)

func (m Metodo) String() string {
	switch m {
	case APIKey:
		return "llave de API (ANTHROPIC_API_KEY)"
	case AuthToken:
		return "token (ANTHROPIC_AUTH_TOKEN)"
	case PerfilOAuth:
		return "perfil de ant auth login (ANTHROPIC_PROFILE)"
	case Federacion:
		return "federación de identidades"
	default:
		return "ninguna"
	}
}

type Estado struct {
	Metodo Metodo
	// Avisos son configuraciones que técnicamente funcionan pero que casi
	// seguro no son lo que la persona quería.
	Avisos []string
}

// Detectar reproduce el orden de precedencia del SDK:
//
//	ANTHROPIC_API_KEY → ANTHROPIC_AUTH_TOKEN → ANTHROPIC_PROFILE → federación
//
// No valida que la credencial sirva, solo cuál se va a usar.
func Detectar() Estado {
	var e Estado

	llave, llaveDefinida := os.LookupEnv("ANTHROPIC_API_KEY")
	token, tokenDefinido := os.LookupEnv("ANTHROPIC_AUTH_TOKEN")
	perfil := os.Getenv("ANTHROPIC_PROFILE")
	fed := estadoFederacion()

	llaveVacia := llaveDefinida && strings.TrimSpace(llave) == ""
	tokenVacio := tokenDefinido && strings.TrimSpace(token) == ""

	switch {
	// Definidas pero vacías siguen ganando la precedencia y no autentican:
	// el resultado es que bloquean todo lo demás en silencio.
	case llaveVacia || tokenVacio:
		e.Metodo = Ninguno
	case llaveDefinida:
		e.Metodo = APIKey
	case tokenDefinido:
		e.Metodo = AuthToken
	case perfil != "":
		e.Metodo = PerfilOAuth
	case fed == federacionCompleta:
		e.Metodo = Federacion
	default:
		e.Metodo = Ninguno
	}

	for nombre, vacia := range map[string]bool{
		"ANTHROPIC_API_KEY":    llaveVacia,
		"ANTHROPIC_AUTH_TOKEN": tokenVacio,
	} {
		if vacia {
			e.Avisos = append(e.Avisos, nombre+" está definida pero vacía. Aun vacía "+
				"tiene prioridad sobre la federación y sobre el perfil de ant, así que "+
				"bloquea todo lo demás sin dar ningún error. Elimínala en lugar de "+
				"dejarla en blanco.")
		}
	}

	if fed == federacionParcial {
		e.Avisos = append(e.Avisos,
			"la federación de identidades está a medio configurar: falta "+
				strings.Join(faltantesFederacion(), ", ")+". Se ignorará por completo.")
	}

	if fed == federacionCompleta && e.Metodo != Federacion && e.Metodo != Ninguno {
		e.Avisos = append(e.Avisos, fmt.Sprintf(
			"hay federación configurada, pero se usará %s porque tiene prioridad. "+
				"Para que la federación entre, elimina esa variable.", e.Metodo))
	}

	return e
}

type nivelFederacion int

const (
	federacionAusente nivelFederacion = iota
	federacionParcial
	federacionCompleta
)

func estadoFederacion() nivelFederacion {
	faltan := faltantesFederacion()
	switch len(faltan) {
	case 0:
		return federacionCompleta
	case len(varsFederacion) + 1: // ninguna de las cuatro está puesta
		return federacionAusente
	default:
		return federacionParcial
	}
}

func faltantesFederacion() []string {
	var faltan []string
	for _, v := range varsFederacion {
		if os.Getenv(v) == "" {
			faltan = append(faltan, v)
		}
	}
	// El token de identidad llega por archivo o por variable; basta con uno.
	if os.Getenv("ANTHROPIC_IDENTITY_TOKEN_FILE") == "" &&
		os.Getenv("ANTHROPIC_IDENTITY_TOKEN") == "" {
		faltan = append(faltan, "ANTHROPIC_IDENTITY_TOKEN_FILE o ANTHROPIC_IDENTITY_TOKEN")
	}
	return faltan
}
