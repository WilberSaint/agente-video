package credenciales

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// CargarEnv lee un archivo .env y define las variables que falten.
//
// Las variables del sistema ganan siempre: un .env es para no tener que
// recordar los comandos de PowerShell, no para pisar lo que alguien puso a
// propósito en la sesión. Ese orden también permite probar una llave distinta
// sin editar el archivo.
//
// Formato: una línea por variable, NOMBRE=valor. Se admiten comentarios con #,
// líneas en blanco, comillas alrededor del valor y el prefijo "export".
func CargarEnv(ruta string) (int, error) {
	f, err := os.Open(ruta)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // no tener .env es lo normal, no un error
		}
		return 0, err
	}
	defer f.Close()

	definidas := 0
	sc := bufio.NewScanner(f)
	linea := 0
	for sc.Scan() {
		linea++
		texto := strings.TrimSpace(strings.TrimPrefix(sc.Text(), string(rune(0xFEFF))))
		if texto == "" || strings.HasPrefix(texto, "#") {
			continue
		}
		texto = strings.TrimPrefix(texto, "export ")

		nombre, valor, ok := strings.Cut(texto, "=")
		if !ok {
			return definidas, fmt.Errorf("%s línea %d: falta el signo igual", ruta, linea)
		}
		nombre = strings.TrimSpace(nombre)
		if nombre == "" {
			return definidas, fmt.Errorf("%s línea %d: nombre vacío", ruta, linea)
		}

		valor = strings.TrimSpace(valor)
		// Quitar comillas envolventes, que la gente pone por costumbre y que
		// acabarían formando parte de la llave.
		if len(valor) >= 2 {
			if (valor[0] == '"' && valor[len(valor)-1] == '"') ||
				(valor[0] == '\'' && valor[len(valor)-1] == '\'') {
				valor = valor[1 : len(valor)-1]
			}
		}

		if _, ya := os.LookupEnv(nombre); ya {
			continue // el entorno real manda
		}
		if err := os.Setenv(nombre, valor); err != nil {
			return definidas, err
		}
		definidas++
	}
	return definidas, sc.Err()
}
