# agente-video

Genera videos verticales cortos (TikTok / Reels / Shorts) a partir de **un tema
y un perfil**: guion, imágenes, narración, subtítulos y montaje, sin intervención
manual.

Corre entero en CPU. El único costo por video son los centavos del guion.

```
tema + perfil
     │
     ├─ 1. guion ........ Claude escribe narración + un prompt visual por escena
     ├─ 2. imágenes ..... una por escena (Pollinations o Cloudflare)
     ├─ 3. voz .......... Piper, local
     ├─ 4. subtítulos ... whisper.cpp, local
     └─ 5. montaje ...... ffmpeg: Ken Burns + transiciones + subs + música
                                    │
                             salida/<perfil>/*.mp4 + *.txt
```

---

## Por qué está diseñado así

**El agente no sabe de temáticas.** Toda la personalidad vive en el perfil: el
estilo visual, la voz, el tono narrativo, el formato, el personaje recurrente.
El mismo binario sirve a diez canales distintos sin tocar una línea de código —
solo se agrega una carpeta en `perfiles/`.

**Cada etapa deja checkpoint en disco.** Si el render falla en el minuto 6, al
reintentar se reutilizan el guion, las imágenes y la voz ya generados. Nunca se
paga dos veces por lo mismo.

**Los proveedores están detrás de interfaces.** Cambiar Ken Burns por Sora, o
Piper por ElevenLabs, es escribir un tipo nuevo que cumpla la interfaz y agregar
un `case` en `construirProveedores`. El pipeline no se entera.

---

## Instalación

Requiere **Go 1.22+**. Todo lo demás lo baja el instalador dentro de `bin/`,
sin tocar el sistema ni el PATH.

```powershell
.\instalar.ps1              # ffmpeg, whisper.cpp + modelo, piper + voz
go build -o agente-video.exe .\cmd\agente-video
.\agente-video.exe doctor   # verifica binarios, modelos y credenciales
```

Opciones del instalador:

```powershell
.\instalar.ps1 -ModeloWhisper small     # más preciso, ~3x más lento
.\instalar.ps1 -Voz es_ES-sharvard-medium
.\instalar.ps1 -Forzar                  # re-descarga todo
```

### Credenciales

| Variable | Para qué | ¿Obligatoria? |
|---|---|---|
| `ANTHROPIC_API_KEY` | el guionista | **sí** |
| `CF_ACCOUNT_ID` / `CF_API_TOKEN` | solo si usas Cloudflare para imágenes | no |
| `WHISPER_MODELO` | ruta a otro `.bin` de whisper | no |

```powershell
$env:ANTHROPIC_API_KEY = "sk-ant-..."          # solo esta sesión
[Environment]::SetEnvironmentVariable("ANTHROPIC_API_KEY","sk-ant-...","User")   # permanente
```

---

## Uso

```powershell
.\agente-video.exe perfiles
.\agente-video.exe generar -perfil demo -tema "el faro que nadie visita"
```

Al terminar quedan dos archivos en `salida/demo/`: el `.mp4` y un `.txt` con
título, descripción y hashtags listos para pegar al publicar.

**Reanudar un trabajo interrumpido** — el id sale del nombre de la carpeta en
`trabajo/<perfil>/`:

```powershell
.\agente-video.exe generar -perfil demo -tema "el faro que nadie visita" `
                           -trabajo 20260831-143022-el-faro-que-nadie-visita
```

Ctrl+C corta limpio: lo ya generado se conserva.

---

## El perfil

Un perfil es una carpeta en `perfiles/<id>/` con un `perfil.json`. Los campos
que cambian el resultado de forma notoria:

| Campo | Efecto |
|---|---|
| `guion.tono` | la voz narrativa. Es lo que más distingue un canal de otro |
| `guion.escenas` | número de imágenes. Más escenas = más ritmo y más render |
| `imagen.estilo` | se añade a **todos** los prompts. Define la identidad visual |
| `imagen.semilla` | fija = escenas más coherentes entre sí; cámbiala para otra veta |
| `imagen.personaje` | descripción física repetida en cada prompt, para personajes recurrentes |
| `video.zoom` | intensidad del Ken Burns. 1.10 es sutil, 1.30 es agresivo |
| `subtitulos.margen_v` | altura de los subtítulos; súbelo para que no los tape la UI de la app |
| `subtitulos.animacion` | cómo aparecen los subtítulos. Ver abajo |

Para crear un perfil nuevo, copia `perfiles/demo/` y edita el `id` y el `nombre`.

---

## Subtítulos animados

whisper entrega tiempos **por palabra**, así que los subtítulos se animan sin
librerías extra: el ASS se genera con etiquetas de override y libass las
renderiza. Cuatro modos, en `subtitulos.animacion`:

| Modo | Qué hace | Cuándo usarlo |
|---|---|---|
| `pop` | La línea entra pequeña, se pasa de tamaño y se asienta | **Predeterminado.** Se ve vivo sin distraer |
| `karaoke` | La línea se queda y se resalta en color la palabra que se dice | Narración densa, donde ayuda leer la frase completa |
| `palabra` | Una sola palabra a la vez, grande y centrada | Máxima retención. El más agresivo |
| `ninguna` | Estáticos | Cuando el contenido manda y el texto solo acompaña |

Se comparan sin editar el perfil, y como solo cambia la etapa 5 el re-render
tarda segundos:

```powershell
Remove-Item trabajo\demo\<id-trabajo>\05-final.mp4
.\agente-video.exe generar -perfil demo -tema "..." -trabajo <id-trabajo> -animacion karaoke
```

Ajustes finos: `escala_pop` (cuánto se pasa el rebote; 112 es discreto, 130
exagerado), `palabras_por_linea` (4 va bien en vertical) y `color_activo` para
karaoke. **Ojo con el color**: en ASS el orden es `&HBBGGRR&`, azul-verde-rojo
al revés de lo habitual — `&H0000E5FF&` es ámbar, no azul.

En modo `palabra` conviene subir `tam_px` bastante (110-140): hay una sola
palabra en pantalla y debe llenar el ancho.

### Sobre la consistencia de personajes

Los generadores de imagen gratuitos no garantizan que el mismo personaje se vea
igual en ocho escenas. Dos mitigaciones, en orden de efectividad:

1. **Evitarlo**: dejar `imagen.personaje` vacío y narrar sobre escenarios y
   atmósfera. Es lo que hace la mayoría de los canales de historias, y el
   resultado es más sólido.
2. **Anclarlo**: escribir en `imagen.personaje` una descripción física detallada
   e invariable. Ayuda, pero no es perfecto.

---

## Estructura

```
cmd/agente-video/       CLI: generar, perfiles, doctor
internal/perfil/        carga y validación de perfiles
internal/proveedor/     las interfaces — el contrato del sistema
    guion/claude.go     guionista (Anthropic SDK, claude-opus-5)
    imagen/             pollinations.go, cloudflare.go
    voz/piper.go        síntesis local
    subtitulos/whisper.go
    video/kenburns.go   el grafo de filtros de ffmpeg
internal/pipeline/      las 5 etapas y sus checkpoints
internal/herramientas/  ejecución de binarios externos, ffprobe
perfiles/               un directorio por canal o persona
trabajo/                checkpoints (ignorado por git)
salida/                 videos terminados (ignorado por git)
bin/                    binarios y modelos (ignorado por git)
```

---

## Pruebas

```powershell
go test ./...                                          # todo
go test ./internal/proveedor/imagen -run EnVivo -v     # pega contra el servicio real
go test -short ./...                                   # salta las que necesitan red
```

---

## Rendimiento

Medido en un Xeon E3-1220 v3 (4 núcleos, sin GPU), video vertical de ~45 s:

| Etapa | Tiempo |
|---|---|
| Guion | ~15 s |
| Imágenes | ~2-3 s cada una |
| Narración | ~30 s |
| Subtítulos | ~15 s |
| **Montaje** | **3-8 min** ← el cuello de botella |

El montaje domina porque `zoompan` es intensivo en CPU. Para acelerarlo:
bajar `factorEscala` en `kenburns.go`, subir el `Preset` de x264, o reducir el
número de escenas. Es una etapa desatendida: la forma correcta de usarlo es
encolar varios videos y dejarlos correr.

---

## Añadir un proveedor nuevo

Ejemplo, un generador de video real (Sora, Kling, Runway) cuando llegue el momento:

1. Crear `internal/proveedor/video/kling.go` con un tipo que cumpla
   `proveedor.Videasta`.
2. Agregar el `case "kling"` en `construirProveedores` (`cmd/agente-video/main.go`).
3. Poner `"proveedor": "kling"` en el `video` del perfil.

Nada más cambia. Los perfiles que sigan en `kenburns` no se enteran.
