package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"agente-video/interfaz"
	"agente-video/internal/herramientas"
	"agente-video/internal/horario"
	"agente-video/internal/perfil"
	"agente-video/internal/pipeline"
	"agente-video/internal/proveedor/video"
	"agente-video/internal/proveedor/voz"
	"agente-video/internal/servidor"
	"agente-video/internal/temas"
	"agente-video/internal/trabajos"
)

func cmdServir(ctx context.Context, args []string) error {
	fs := flagSet("servir")
	puerto := fs.Int("puerto", 8787, "puerto donde escuchar")
	direccion := fs.String("direccion", "127.0.0.1", "interfaz de red")
	dirPerfiles := fs.String("perfiles", "perfiles", "carpeta de perfiles")
	dirTrabajos := fs.String("trabajos", "trabajo", "carpeta de checkpoints")
	dirSalida := fs.String("salida", "salida", "carpeta de salida")
	dirBin := fs.String("bin", "bin", "carpeta de binarios")
	if err := fs.Parse(args); err != nil {
		return err
	}
	herramientas.DirBin = *dirBin

	// El ejecutor traduce un trabajo de la cola en una corrida del pipeline.
	// La cola no sabe nada de perfiles ni proveedores; solo de estados.
	ejecutor := func(ctx context.Context, t *trabajos.Trabajo,
		avance func(pipeline.Avance), registrar func(string)) (*trabajos.Resultado, error) {

		p, err := perfil.Cargar(*dirPerfiles, t.Perfil)
		if err != nil {
			return nil, err
		}
		provs, err := construirProveedores(p)
		if err != nil {
			return nil, err
		}

		reg := func(f string, a ...any) {
			linea := fmt.Sprintf("%s  %s", time.Now().Format("15:04:05"), fmt.Sprintf(f, a...))
			registrar(linea)
			log.Print(linea)
		}
		conectarAvisos(provs, reg)

		pl := pipeline.Nuevo(provs, pipeline.Opciones{
			DirTrabajo: *dirTrabajos,
			DirSalida:  *dirSalida,
			Registro:   reg,
			Avance:     avance,
		})

		res, err := pl.Ejecutar(ctx, p, t.Tema, t.Carpeta)
		if err != nil {
			return nil, err
		}
		return &trabajos.Resultado{
			Titulo: res.Guion.Titulo,
			Video:  res.VideoFinal,
			Textos: strings.TrimSuffix(res.VideoFinal, ".mp4") + ".txt",
			Publicacion: trabajos.Publicacion{
				Titulo:      res.Guion.TituloPublicable(),
				Descripcion: res.Guion.DescripcionPublicable(),
			},
		}, nil
	}

	cola := trabajos.NuevaCola(filepath.Join(*dirTrabajos, "cola.json"), ejecutor)
	cola.Arrancar(ctx)

	// Banco de temas y horario: lo que permite producir sin nadie delante.
	banco := temas.Nuevo(filepath.Join(*dirTrabajos, "temas.json"))
	h := horario.Nuevo(filepath.Join(*dirTrabajos, "horario.json"),
		disparador(banco, cola, *dirPerfiles))
	h.Arrancar(ctx)

	panel := interfaz.FS()
	srv := servidor.Nuevo(cola, *dirPerfiles, panel, banco, h)

	dir := fmt.Sprintf("%s:%d", *direccion, *puerto)
	http := &http.Server{
		Addr:    dir,
		Handler: srv.Rutas(),
		// Sin límite de escritura: el flujo de eventos vive tanto como el
		// navegador esté abierto, y cualquier tope lo cortaría a media
		// generación.
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("panel en http://%s\n", dir)
	if panel == nil {
		fmt.Println("aviso: la interfaz no está compilada; la API funciona pero no hay panel.")
		fmt.Println("       cd interfaz && npm install && npm run build")
	}
	if *direccion == "127.0.0.1" {
		fmt.Println("solo accesible desde esta máquina. Para abrirlo a la red: -direccion 0.0.0.0")
	}
	fmt.Println("Ctrl+C para parar.")

	go func() {
		<-ctx.Done()
		cierre, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelar()
		_ = http.Shutdown(cierre)
		cola.Esperar(20 * time.Second)
		h.Esperar(5 * time.Second)
	}()

	if err := http.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
		return err
	}
	return nil
}

// conectarAvisos hace que los avisos del render salgan por el mismo canal que
// el resto del progreso, para que se vean en el panel y no solo en la consola.
func conectarAvisos(provs pipeline.Proveedores, reg func(string, ...any)) {
	if kb, ok := provs.Videasta.(*video.KenBurns); ok {
		kb.Aviso = func(f string, a ...any) { reg("      aviso: "+f, a...) }
	}
	// El locutor de pago va envuelto en el respaldo, así que hay que entrar una
	// capa: si no, un cambio de proveedor a mitad de lote no se vería en el panel.
	if cr, ok := provs.Locutor.(*voz.ConRespaldo); ok {
		cr.Aviso = func(f string, a ...any) { reg("      aviso: "+f, a...) }
		if el, ok := cr.Principal.(*voz.ElevenLabs); ok {
			el.Aviso = func(f string, a ...any) { reg("      aviso: "+f, a...) }
		}
	}
}
