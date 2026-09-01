package credenciales

import (
	"os"
	"path/filepath"
	"testing"
)

func escribirEnv(t *testing.T, contenido string) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(ruta, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// desdefinir quita una variable durante la prueba y la restaura al terminar.
func desdefinir(t *testing.T, nombre string) {
	t.Helper()
	t.Setenv(nombre, "")
	if err := os.Unsetenv(nombre); err != nil {
		t.Fatal(err)
	}
}

func TestFormatosAceptados(t *testing.T) {
	for _, n := range []string{"PRUEBA_A", "PRUEBA_B", "PRUEBA_C", "PRUEBA_D", "PRUEBA_E"} {
		desdefinir(t, n)
	}
	ruta := escribirEnv(t, `
# un comentario
PRUEBA_A=simple

PRUEBA_B="entre comillas dobles"
PRUEBA_C='entre comillas simples'
export PRUEBA_D=con-export
PRUEBA_E=  con espacios alrededor
`)
	n, err := CargarEnv(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("definidas = %d, esperaba 5", n)
	}
	quiero := map[string]string{
		"PRUEBA_A": "simple",
		"PRUEBA_B": "entre comillas dobles",
		"PRUEBA_C": "entre comillas simples",
		"PRUEBA_D": "con-export",
		"PRUEBA_E": "con espacios alrededor",
	}
	for nombre, valor := range quiero {
		if got := os.Getenv(nombre); got != valor {
			t.Errorf("%s = %q, esperaba %q", nombre, got, valor)
		}
	}
}

// Lo que define la precedencia: el .env es comodidad, no autoridad. Poder
// probar otra llave sin editar el archivo depende de esto.
func TestElEntornoRealTienePrioridad(t *testing.T) {
	t.Setenv("PRUEBA_PRIORIDAD", "la del sistema")
	ruta := escribirEnv(t, "PRUEBA_PRIORIDAD=la del archivo\n")

	n, err := CargarEnv(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("definidas = %d; no debería sobrescribir lo que ya existe", n)
	}
	if got := os.Getenv("PRUEBA_PRIORIDAD"); got != "la del sistema" {
		t.Errorf("valor = %q; el .env pisó al entorno", got)
	}
}

// Una llave con signos igual dentro no se debe partir por el segundo.
func TestValorConSignoIgual(t *testing.T) {
	desdefinir(t, "PRUEBA_IGUAL")
	ruta := escribirEnv(t, "PRUEBA_IGUAL=abc=def=ghi\n")
	if _, err := CargarEnv(ruta); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("PRUEBA_IGUAL"); got != "abc=def=ghi" {
		t.Errorf("valor = %q", got)
	}
}

// No tener .env es lo normal, no un fallo.
func TestSinArchivoNoEsError(t *testing.T) {
	n, err := CargarEnv(filepath.Join(t.TempDir(), "no-existe"))
	if err != nil {
		t.Errorf("devolvió error: %v", err)
	}
	if n != 0 {
		t.Errorf("definidas = %d, esperaba 0", n)
	}
}

func TestLineaMalFormadaSeReporta(t *testing.T) {
	ruta := escribirEnv(t, "esto no tiene igual\n")
	if _, err := CargarEnv(ruta); err == nil {
		t.Error("aceptó una línea sin signo igual")
	}
}
