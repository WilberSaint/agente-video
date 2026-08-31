package credenciales

import (
	"os"
	"strings"
	"testing"
)

// limpiar deja el entorno sin ninguna credencial, para que cada caso parta de
// cero sin importar cómo esté configurada la máquina que corre las pruebas.
//
// Ojo: t.Setenv(v, "") DEFINE la variable vacía, que es precisamente el caso
// que este paquete detecta como problema. Para desdefinirla de verdad hace
// falta os.Unsetenv, y se llama a t.Setenv antes solo para que el framework
// registre la restauración al terminar la prueba.
func limpiar(t *testing.T) {
	t.Helper()
	for _, v := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_PROFILE",
		"ANTHROPIC_FEDERATION_RULE_ID", "ANTHROPIC_ORGANIZATION_ID",
		"ANTHROPIC_SERVICE_ACCOUNT_ID", "ANTHROPIC_IDENTITY_TOKEN_FILE",
		"ANTHROPIC_IDENTITY_TOKEN",
	} {
		t.Setenv(v, "")
		if err := os.Unsetenv(v); err != nil {
			t.Fatal(err)
		}
	}
}

func federar(t *testing.T) {
	t.Helper()
	t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "regla")
	t.Setenv("ANTHROPIC_ORGANIZATION_ID", "org")
	t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "cuenta")
	t.Setenv("ANTHROPIC_IDENTITY_TOKEN_FILE", "/var/run/token")
}

func TestSinNadaConfigurado(t *testing.T) {
	limpiar(t)
	if e := Detectar(); e.Metodo != Ninguno {
		t.Errorf("método = %v, esperaba Ninguno", e.Metodo)
	}
}

func TestFederacionCompleta(t *testing.T) {
	limpiar(t)
	federar(t)
	e := Detectar()
	if e.Metodo != Federacion {
		t.Fatalf("método = %v, esperaba Federacion", e.Metodo)
	}
	if len(e.Avisos) != 0 {
		t.Errorf("no esperaba avisos, obtuve %v", e.Avisos)
	}
}

func TestFederacionParcialAvisa(t *testing.T) {
	limpiar(t)
	t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "regla")
	t.Setenv("ANTHROPIC_ORGANIZATION_ID", "org")
	// faltan la cuenta de servicio y el token

	e := Detectar()
	if e.Metodo != Ninguno {
		t.Errorf("método = %v, esperaba Ninguno con federación incompleta", e.Metodo)
	}
	if len(e.Avisos) == 0 {
		t.Fatal("esperaba un aviso por federación a medias")
	}
	if !strings.Contains(e.Avisos[0], "ANTHROPIC_SERVICE_ACCOUNT_ID") {
		t.Errorf("el aviso no nombra lo que falta: %q", e.Avisos[0])
	}
}

// El caso que motiva todo este paquete.
func TestLlaveVaciaBloqueaLaFederacion(t *testing.T) {
	limpiar(t)
	federar(t)
	t.Setenv("ANTHROPIC_API_KEY", "")

	e := Detectar()
	if e.Metodo != Ninguno {
		t.Errorf("método = %v, esperaba Ninguno: una llave vacía no autentica", e.Metodo)
	}
	if len(e.Avisos) == 0 {
		t.Fatal("esperaba aviso: una llave vacía anula la federación en silencio")
	}
	if !strings.Contains(e.Avisos[0], "vacía") {
		t.Errorf("aviso poco claro: %q", e.Avisos[0])
	}
}

func TestLlaveTienePrioridadSobreFederacion(t *testing.T) {
	limpiar(t)
	federar(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-loquesea")

	e := Detectar()
	if e.Metodo != APIKey {
		t.Fatalf("método = %v, esperaba APIKey", e.Metodo)
	}
	if len(e.Avisos) == 0 {
		t.Fatal("esperaba aviso: la federación configurada se está ignorando")
	}
	if !strings.Contains(e.Avisos[0], "prioridad") {
		t.Errorf("aviso poco claro: %q", e.Avisos[0])
	}
}

func TestOrdenDePrecedencia(t *testing.T) {
	limpiar(t)
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "tok")
	t.Setenv("ANTHROPIC_PROFILE", "trabajo")

	if e := Detectar(); e.Metodo != AuthToken {
		t.Errorf("método = %v, esperaba AuthToken por encima del perfil", e.Metodo)
	}
}
