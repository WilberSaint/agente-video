# instalar.ps1 — descarga los binarios y modelos que el agente necesita.
#
# Este servidor no tiene winget ni choco, así que todo se baja a mano dentro de
# bin/. Nada se instala en el sistema ni toca el PATH: si quieres empezar de
# cero, borra la carpeta bin/ y vuelve a correr esto.
#
#   .\instalar.ps1              descarga todo lo que falte
#   .\instalar.ps1 -Forzar      vuelve a descargar aunque ya exista

[CmdletBinding()]
param(
    [switch]$Forzar,
    # Voz por defecto. Otras: es_ES-sharvard-medium, es_MX-ald-medium
    [string]$Voz = "es_MX-claude-high",
    # Modelo de whisper: base (rápido) | small (más preciso, ~3x más lento)
    [ValidateSet("base", "small", "medium")]
    [string]$ModeloWhisper = "base"
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"   # acelera muchísimo Invoke-WebRequest

$Raiz    = $PSScriptRoot
$Bin     = Join-Path $Raiz "bin"
$Voces   = Join-Path $Bin  "voces"
$Modelos = Join-Path $Bin  "modelos"
$Temp    = Join-Path $Bin  ".descargas"

foreach ($d in @($Bin, $Voces, $Modelos, $Temp)) {
    if (-not (Test-Path $d)) { New-Item -ItemType Directory -Path $d -Force | Out-Null }
}

function Necesita([string]$ruta) {
    if ($Forzar) { return $true }
    -not (Test-Path $ruta)
}

# Firmas de archivo esperadas. Un antivirus o proxy corporativo que intercepte
# HTTPS devuelve una pagina HTML de bloqueo con codigo 200: sin esta validacion
# el zip "se descarga" y revienta mucho despues, con un error incomprensible.
$script:Firmas = @{
    ".zip"  = [byte[]](0x50, 0x4B)          # PK
    ".onnx" = [byte[]](0x08)                # protobuf
    ".bin"  = [byte[]](0x6C, 0x6D, 0x67, 0x67) # 'lmgg' (ggml)
}

function Validar([string]$ruta, [string]$url, [string]$etiqueta) {
    $ext = [IO.Path]::GetExtension($ruta)
    if (-not $script:Firmas.ContainsKey($ext)) { return }

    $esperada = $script:Firmas[$ext]
    $real = [byte[]](Get-Content -Path $ruta -Encoding Byte -TotalCount $esperada.Length)
    for ($i = 0; $i -lt $esperada.Length; $i++) {
        if ($real[$i] -ne $esperada[$i]) {
            $primeros = [Text.Encoding]::ASCII.GetString($real)
            Remove-Item $ruta -Force -ErrorAction SilentlyContinue
            throw @"
$etiqueta se descargo pero NO es un archivo valido.

  url      : $url
  esperaba : $ext
  recibi   : algo que empieza con '$primeros'

Casi siempre significa que un antivirus o proxy interceptó la conexión y
devolvió su propia página de bloqueo. En este servidor, Kaspersky Small Office
Security bloquea algunos dominios de descarga.

Qué hacer: abre esa URL en un navegador de la máquina. Si ves una pantalla de
Kaspersky, agrega el dominio a la lista de confianza (Configuración → Red →
Dominios de confianza), o baja el archivo a mano y déjalo en bin\.
"@
        }
    }
}

function Bajar([string]$url, [string]$destino, [string]$etiqueta) {
    Write-Host "  bajando $etiqueta..." -NoNewline
    try {
        Invoke-WebRequest -Uri $url -OutFile $destino -UseBasicParsing -MaximumRedirection 10
    } catch {
        Write-Host " FALLO"
        # $_ puede traer una pagina HTML completa; nos quedamos con el encabezado.
        $msg = ($_.Exception.Message -split "`n")[0]
        throw "no se pudo descargar $etiqueta desde $url : $msg"
    }
    Validar $destino $url $etiqueta
    $mb = [math]::Round((Get-Item $destino).Length / 1MB, 1)
    Write-Host " ok ($mb MB)"
}

# Resuelve un asset de la ultima release de un repo de GitHub.
function AssetGitHub([string]$repo, [string]$nombreAsset) {
    $rel = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" `
                             -Headers @{ "User-Agent" = "agente-video" }
    $asset = $rel.assets | Where-Object { $_.name -eq $nombreAsset } | Select-Object -First 1
    if (-not $asset) { throw "la release $($rel.tag_name) de $repo no trae $nombreAsset" }
    return [pscustomobject]@{ Url = $asset.browser_download_url; Tag = $rel.tag_name }
}

# Copia un archivo desde un zip extraído, buscándolo recursivamente.
function Extraer([string]$origenDir, [string]$patron, [string]$destinoDir) {
    $encontrados = Get-ChildItem -Path $origenDir -Filter $patron -Recurse -File
    if ($encontrados.Count -eq 0) { throw "no se encontró '$patron' dentro del zip" }
    foreach ($f in $encontrados) {
        Copy-Item $f.FullName -Destination (Join-Path $destinoDir $f.Name) -Force
    }
    return $encontrados.Count
}

Write-Host ""
Write-Host "instalando dependencias de agente-video en $Bin"
Write-Host ""

# ---------------------------------------------------------------- ffmpeg -----
if (Necesita (Join-Path $Bin "ffmpeg.exe")) {
    Write-Host "[1/4] ffmpeg + ffprobe"
    # Se usan los builds de BtbN en GitHub: gyan.dev lo bloquea Kaspersky en
    # esta maquina. La variante "gpl" (sin -shared) es estatica, un solo .exe.
    $a = AssetGitHub "BtbN/FFmpeg-Builds" "ffmpeg-master-latest-win64-gpl.zip"
    $zip = Join-Path $Temp "ffmpeg.zip"
    $out = Join-Path $Temp "ffmpeg"
    Bajar $a.Url $zip "ffmpeg"
    if (Test-Path $out) { Remove-Item $out -Recurse -Force }
    Expand-Archive -Path $zip -DestinationPath $out -Force
    Extraer $out "ffmpeg.exe"  $Bin | Out-Null
    Extraer $out "ffprobe.exe" $Bin | Out-Null
    Write-Host "      listo"
} else {
    Write-Host "[1/4] ffmpeg ya presente"
}

# ----------------------------------------------------------- whisper.cpp -----
if (Necesita (Join-Path $Bin "whisper-cli.exe")) {
    Write-Host "[2/4] whisper.cpp"
    # El tag cambia seguido; lo resolvemos contra la API en vez de fijarlo.
    $a = AssetGitHub "ggml-org/whisper.cpp" "whisper-bin-x64.zip"
    $zip = Join-Path $Temp "whisper.zip"
    $out = Join-Path $Temp "whisper"
    Bajar $a.Url $zip "whisper.cpp $($a.Tag)"
    if (Test-Path $out) { Remove-Item $out -Recurse -Force }
    Expand-Archive -Path $zip -DestinationPath $out -Force

    # Se lleva también los .dll: sin ellos el .exe no arranca.
    Extraer $out "whisper-cli.exe" $Bin | Out-Null
    $dlls = Extraer $out "*.dll" $Bin
    Write-Host "      listo (whisper-cli.exe + $dlls dll)"
} else {
    Write-Host "[2/4] whisper.cpp ya presente"
}

# --------------------------------------------------- modelo de whisper -------
$rutaModelo = Join-Path $Modelos "ggml-$ModeloWhisper.bin"
if (Necesita $rutaModelo) {
    Write-Host "[3/4] modelo de transcripcion ggml-$ModeloWhisper"
    Bajar "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-$ModeloWhisper.bin" `
          $rutaModelo "ggml-$ModeloWhisper.bin"
} else {
    Write-Host "[3/4] modelo ggml-$ModeloWhisper ya presente"
}

# ------------------------------------------------------- piper + voz ---------
if (Necesita (Join-Path $Bin "piper.exe")) {
    Write-Host "[4/4] piper (voz)"
    $zip = Join-Path $Temp "piper.zip"
    $out = Join-Path $Temp "piper"
    Bajar "https://github.com/rhasspy/piper/releases/download/2023.11.14-2/piper_windows_amd64.zip" `
          $zip "piper"
    if (Test-Path $out) { Remove-Item $out -Recurse -Force }
    Expand-Archive -Path $zip -DestinationPath $out -Force
    # piper viene con su propio runtime de onnx; copiamos la carpeta completa.
    $dir = Get-ChildItem -Path $out -Directory | Select-Object -First 1
    $origen = if ($dir) { $dir.FullName } else { $out }
    Copy-Item (Join-Path $origen "*") -Destination $Bin -Recurse -Force
    Write-Host "      listo"
} else {
    Write-Host "[4/4] piper ya presente"
}

# La voz vive aparte del binario: un .onnx y su .json de configuracion.
$idioma = ($Voz -split "-")[0]          # es_MX
$region = ($idioma -split "_")[0]       # es
$locutor = ($Voz -split "-")[1]         # claude
$calidad = ($Voz -split "-")[2]         # high
$baseVoz = "https://huggingface.co/rhasspy/piper-voices/resolve/main/$region/$idioma/$locutor/$calidad/$Voz"

foreach ($ext in @(".onnx", ".onnx.json")) {
    $destino = Join-Path $Voces "$Voz$ext"
    if (Necesita $destino) {
        Bajar "$baseVoz$ext" $destino "voz $Voz$ext"
    }
}

Remove-Item $Temp -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "instalacion terminada. Verifica con:"
Write-Host "    .\agente-video.exe doctor"
Write-Host ""
