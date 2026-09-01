package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"agente-video/internal/horario"
	"agente-video/internal/perfil"
	"agente-video/internal/proveedor/guion"
	"agente-video/internal/temas"
	"agente-video/internal/trabajos"
)

// atascoMaximo permite que una tanda normal siga en curso cuando entra la
// siguiente regla —dos videos tardan media hora larga— pero corta antes de que
// se acumulen noches enteras.
const atascoMaximo = 4

// disparador convierte una regla del horario en videos encolados.
//
// Aquí es donde el agente deja de ser una herramienta: nadie está delante, así
// que cada decisión debe poder tomarse sola y dejar dicho qué pasó. El resumen
// que devuelve se guarda en la regla, porque lo primero que se quiere saber por
// la mañana es si trabajó y con qué.
func disparador(banco *temas.Banco, cola *trabajos.Cola, dirPerfiles string) horario.Disparar {
	return func(ctx context.Context, r horario.Regla) string {
		log.Printf("horario: disparando %s — %d video(s) de %q", r.ID, r.Cantidad, r.Perfil)

		// Con un solo obrero, encolar sobre una cola atascada no adelanta nada:
		// solo entierra el problema bajo más trabajos y consume los temas del
		// banco sin producir un video. Mejor saltarse la noche y dejarlo dicho.
		if pendientes := cola.Atasco(); pendientes >= atascoMaximo {
			log.Printf("horario: %d trabajo(s) sin terminar; no se encola nada", pendientes)
			return fmt.Sprintf("saltado: la cola arrastra %d trabajo(s) sin terminar", pendientes)
		}

		tomados := banco.Tomar(r.Perfil, r.Cantidad)

		// Si el banco no da para la tanda y la regla lo permite, se piden ideas
		// nuevas. Sin esto, un banco vacío significa una noche perdida.
		if len(tomados) < r.Cantidad && r.ProponerSiFaltan {
			faltan := r.Cantidad - len(tomados)
			if n, err := proponerTemas(ctx, banco, dirPerfiles, r.Perfil, faltan); err != nil {
				log.Printf("horario: no se pudieron proponer temas: %v", err)
			} else if n > 0 {
				tomados = append(tomados, banco.Tomar(r.Perfil, faltan)...)
			}
		}

		if len(tomados) == 0 {
			return "sin temas pendientes; no se encoló nada"
		}

		encolados := 0
		for _, t := range tomados {
			trabajo, err := cola.Encolar(r.Perfil, t.Texto)
			if err != nil {
				// El tema vuelve a pendiente: la idea sigue siendo buena, lo
				// que falló fue la máquina.
				banco.Devolver(t.ID)
				log.Printf("horario: no se pudo encolar %q: %v", t.Texto, err)
				continue
			}
			banco.MarcarTrabajo(t.ID, trabajo.ID)
			encolados++
		}

		resumen := fmt.Sprintf("%d video(s) encolados", encolados)
		if encolados < r.Cantidad {
			resumen += fmt.Sprintf(" de %d pedidos (faltaron temas)", r.Cantidad)
		}
		log.Printf("horario: %s", resumen)
		return resumen
	}
}

// proponerTemas pide ideas nuevas al guionista y las mete en el banco.
func proponerTemas(ctx context.Context, banco *temas.Banco, dirPerfiles,
	idPerfil string, cuantos int) (int, error) {

	p, err := perfil.Cargar(dirPerfiles, idPerfil)
	if err != nil {
		return 0, err
	}

	// Se piden algunos de más: parte se descartará por repetir ideas ya usadas,
	// y quedarse corto obligaría a otra llamada.
	c := guion.NuevoClaude(credencialAPI(), credencialWorkspace(), "")
	propuestos, err := c.Proponer(ctx, p, cuantos+3, banco.UsadosRecientes(idPerfil, 40))
	if err != nil {
		return 0, err
	}

	n := banco.Agregar(propuestos, idPerfil, "propuesto")
	log.Printf("horario: %d tema(s) propuestos, %d nuevos", len(propuestos), n)
	return n, nil
}

// Los temas propuestos se marcan como tales para poder distinguir, al mirar el
// banco, cuánto se está apoyando el canal en ideas propias.
func resumirTemas(ts []*temas.Tema) string {
	var textos []string
	for _, t := range ts {
		textos = append(textos, t.Texto)
	}
	return strings.Join(textos, " · ")
}

// credencialAPI y credencialWorkspace centralizan de dónde salen, para que el
// disparador no tenga que conocer los nombres de las variables.
func credencialAPI() string       { return os.Getenv("ANTHROPIC_API_KEY") }
func credencialWorkspace() string { return os.Getenv("ANTHROPIC_WORKSPACE_ID") }
