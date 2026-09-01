// Package interfaz empotra el panel web compilado dentro del binario.
//
// Vive aquí y no en cmd/ porque las rutas de go:embed se resuelven relativas al
// archivo fuente que lleva la directiva, no a la raíz del módulo.
//
// Gracias a esto, desplegar sigue siendo copiar un .exe: no hay carpeta dist/
// que sincronizar, ni forma de que la interfaz y la API queden desfasadas.
package interfaz

import (
	"embed"
	"io/fs"
)

// El patrón "all:" incluye los archivos que empiezan por punto o guion bajo,
// que Vite genera y que go:embed omitiría por defecto.
//
//go:embed all:dist
var empotrada embed.FS

// FS devuelve el panel listo para servir, o nil si todavía no se ha compilado
// la interfaz. Devolver nil en vez de fallar permite usar la API sin Node
// instalado, que es útil en un servidor donde solo interesa la cola.
func FS() fs.FS {
	sub, err := fs.Sub(empotrada, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil // solo está el marcador: nadie ha corrido npm run build
	}
	return sub
}
