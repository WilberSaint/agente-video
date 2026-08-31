# generar-efectos.ps1 — sintetiza los efectos de sonido con ffmpeg.
#
# Se generan en vez de descargarse por dos razones: no dependen de la licencia
# de nadie, y el resultado es reproducible en cualquier máquina con solo correr
# este script. Los .wav resultantes están en .gitignore.
#
#   .\generar-efectos.ps1

$ErrorActionPreference = "Stop"
$ffmpeg = Join-Path $PSScriptRoot "bin\ffmpeg.exe"
$destino = Join-Path $PSScriptRoot "assets\efectos"

if (-not (Test-Path $ffmpeg)) { throw "no se encontró $ffmpeg; corre primero .\instalar.ps1" }
if (-not (Test-Path $destino)) { New-Item -ItemType Directory -Path $destino -Force | Out-Null }

# Cada efecto es una fuente sintética de lavfi más una envolvente de volumen.
# La envolvente es lo que separa un efecto de un ruido: define ataque y caída.
$efectos = @(
    @{
        nombre = "whoosh"
        desc   = "barrido de aire para cambios de plano"
        # Ruido rosa filtrado, con ataque rápido y caída larga.
        args   = @(
            "-f", "lavfi", "-i", "anoisesrc=d=0.45:c=pink:a=0.9:r=44100",
            "-af", "highpass=f=280,lowpass=f=5500,volume='min(t/0.09,1)*max(0,1-(t-0.09)/0.36)':eval=frame,afade=t=out:st=0.40:d=0.05"
        )
    },
    @{
        nombre = "impacto"
        desc   = "golpe grave para subrayar una revelación"
        # Seno bajo con caída exponencial: el clásico thud de subrayado.
        args   = @(
            "-f", "lavfi", "-i", "aevalsrc='0.8*sin(2*PI*68*t)*exp(-7*t)':d=0.6:s=44100",
            "-af", "lowpass=f=200,volume=1.0"
        )
    },
    @{
        nombre = "riser"
        desc   = "tensión ascendente para el gancho inicial"
        # Barrido de frecuencia creciente y cuadrático, con entrada suave.
        args   = @(
            "-f", "lavfi", "-i", "aevalsrc='0.35*sin(2*PI*(140+900*t*t)*t)':d=1.2:s=44100",
            "-af", "volume='min(t/0.25,1)*min(1,(1.2-t)/0.15)':eval=frame"
        )
    },
    @{
        nombre = "clic"
        desc   = "marca seca y corta para datos o cifras"
        args   = @(
            "-f", "lavfi", "-i", "anoisesrc=d=0.08:c=white:a=0.5:r=44100",
            "-af", "highpass=f=1800,volume='max(0,1-t/0.06)':eval=frame"
        )
    }
)

Write-Host ""
foreach ($e in $efectos) {
    $salida = Join-Path $destino "$($e.nombre).wav"
    & $ffmpeg -y -loglevel error @($e.args) -c:a pcm_s16le $salida
    if ($LASTEXITCODE -ne 0) { throw "falló la generación de $($e.nombre)" }

    # Comprobamos que no salió en silencio: una envolvente mal escrita produce
    # un archivo válido y completamente mudo, que no da ningún error.
    #
    # ffmpeg escribe volumedetect en stderr, y en PowerShell 5.1 redirigir el
    # stderr de un ejecutable nativo con 2>&1 lo convierte en NativeCommandError
    # y aborta el script aunque el comando haya salido en 0. Por eso se recoge
    # a un archivo temporal.
    $tmp = [IO.Path]::GetTempFileName()
    Start-Process -FilePath $ffmpeg -NoNewWindow -Wait -RedirectStandardError $tmp `
        -ArgumentList @("-i", $salida, "-af", "volumedetect", "-f", "null", "-")
    $pico = (Get-Content $tmp | Select-String "max_volume") -replace '.*max_volume:\s*', ''
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue

    if ([string]::IsNullOrWhiteSpace($pico)) { $pico = "?" }
    if ($pico -match '^-91') { throw "$($e.nombre) salió en silencio; revisa su envolvente" }

    $seg = [math]::Round((Get-Item $salida).Length / 44100 / 2, 2)
    Write-Host ("  {0,-9} {1,5}s  pico {2}" -f $e.nombre, $seg, $pico)
    Write-Host ("            {0}" -f $e.desc) -ForegroundColor DarkGray
}

Write-Host ""
Write-Host "efectos generados en assets\efectos\"
Write-Host ""
