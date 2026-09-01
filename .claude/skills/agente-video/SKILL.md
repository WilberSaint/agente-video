---
name: agente-video
description: Contexto y flujos de trabajo del agente que genera videos verticales cortos (guion, imágenes, voz, subtítulos, montaje). Úsala al trabajar en este repo, al escribir guiones para él, al añadir proveedores o al diagnosticar por qué un video salió mal.
---

# agente-video

Genera videos verticales cortos desde un tema: guion → imágenes fijas → voz →
subtítulos → montaje. Go, un solo `.exe`, binarios externos por ruta.

## La restricción que lo explica todo

**Las imágenes son fijas.** No hay video generado, hay fotos con zoom lento
(Ken Burns) y transiciones. De ahí salen casi todas las decisiones:

- Una narración nunca puede depender de ver movimiento («mira cómo corre»).
- Cada imagen tiene que sostenerse sola y decir lo que se está narrando.
- El ritmo lo marca la voz; las imágenes se reparten según su parte del relato.

Si vas a escribir prompts o guiones, esto manda sobre cualquier otra
consideración estética.

## Arquitectura

Cinco interfaces en `internal/proveedor/proveedor.go`: `Guionista`,
`Imagenero`, `Locutor`, `Subtitulador`, `Videasta`. Cambiar de servicio nunca
toca el pipeline; se añade un `case` en `construirProveedores`
(`cmd/agente-video/main.go`).

**Checkpoints por etapa en disco** (`trabajo/<perfil>/<id>/`): `01-guion.json`,
`02-imagenes/`, `03-voz.wav`, `04-palabras.srt`, `05-final.mp4`. Si el archivo
existe, la etapa se salta. Un fallo en el render no vuelve a pagar el guion ni
a regenerar imágenes. **Aprovecha esto al depurar**: borra solo el checkpoint
de la etapa que quieras rehacer.

**Los perfiles son JSON** (`perfiles/<id>/perfil.json`). El código no sabe nada
de temáticas: canal, estilo, voz, formato y personaje viven ahí.

## Cosas que ya se aprendieron por las malas

- **Los subtítulos los escribe el guion, no whisper.** Whisper transcribe sin
  saber qué se dijo y se equivoca ("su rija" por "su hija"). Su transcripción
  solo aporta los tiempos; las palabras vienen del guion, alineando las dos
  secuencias (`internal/proveedor/video/corregir.go`).
- **Cada guion se escribe sin ver los anteriores.** Por eso cualquier frase que
  no dependa del contenido —un cierre tipo «cuéntame qué habrías hecho tú», un
  arranque tipo «te lo cuento como me lo contaron»— sale idéntica una y otra
  vez. Si aparece una fórmula repetida, se prohíbe en el prompt de
  `internal/proveedor/guion/claude.go`, no se parchea después.
- **El filtro `subtitles=` de ffmpeg usa un lienzo fijo de 384x288.** Por eso
  el ASS se genera aquí, con `PlayResX/Y` del video real.
- **Una etiqueta de salida de ffmpeg se consume UNA vez.** Reutilizar una
  expresión del personaje exige `split=N`.
- **Los generadores gratuitos ignoran la resolución pedida**: Pollinations
  devuelve 576x1024 y Cloudflare 1024x1024 cuadrado. El zoom luego los amplía
  casi 3x. El proveedor `local` (GPU propia) existe por esto.
- **Pollinations admite una petición por IP a la vez.** Dos videos en paralelo
  se tumban con 429.
- Para montarlo sobre una GPU propia está **[GPU.md](../../../GPU.md)**, con los
  números ya calculados para 6 GB de VRAM.
- **Cloudflare no respeta la semilla**, así que no mantiene personajes.
  `SoportaSemilla()` lo declara y el pipeline avisa.

## Flujos habituales

```powershell
# panel web (tiene que quedarse corriendo)
.\agente-video.exe servir              # http://127.0.0.1:8787

# un video suelto
.\agente-video.exe generar -perfil historias -tema "..."

# sin gastar créditos de Claude ni de imágenes de pago
.\agente-video.exe generar -perfil demo -tema "..." -simular

# con un guion ya escrito y/o un audio ya narrado
.\agente-video.exe generar -perfil historias -guion g.json -voz-archivo v.mp3

# overrides por video, para no duplicar perfiles
-voz "elevenlabs:ID"   -animacion pop   -sin-personaje

# comparar guiones sin renderizar (3 llamadas en vez de 40 minutos)
go run .\cmd\probar-guion -guardar guiones historias "tema" "otro" "tercero"
```

## Al escribir un guion a mano

El JSON tiene que cumplir el esquema de `GuionGenerado`: `titulo`,
`descripcion`, `hashtags` (sin almohadilla; `#viral` lo añade el código),
`escenas[].narracion` y `escenas[].planos[]` con `prompt` **en inglés**,
`encuadre` y `sujeto`. El `sujeto` es lo que comparte semilla entre planos: dos
planos con el mismo sujeto salen parecidos.

Cuenta ~150 palabras por minuto de narración y respeta `guion.escenas` del
perfil.

## Reglas de trabajo en este repo

- **Comprobar antes de afirmar.** Aquí se han diagnosticado mal cosas por no
  medir: mide, ejecuta, mira el archivo.
- **Tests en verde antes de subir.** Ha fallado ya y es un fallo de proceso.
- **Comentarios que expliquen el porqué**, no el qué. Los que hay dicen qué
  problema real resolvieron.
- **Nada de secretos en el repo.** Las llaves van en `.env`, que está
  ignorado. No imprimas su valor: longitud y últimos caracteres bastan.
- El proyecto está **en español**: nombres, comentarios y mensajes.
