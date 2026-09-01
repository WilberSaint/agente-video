# instalar-servicio.ps1 — hace que el panel arranque solo con Windows.
#
# Sin esto, el agente solo produce mientras haya una ventana de PowerShell
# abierta. Un horario a las tres de la mañana no sirve de nada si el programa se
# apagó al cerrar sesión o al reiniciar el servidor.
#
# Se usa una tarea programada y no un servicio de Windows porque un ejecutable
# normal no habla el protocolo de servicios: registrarlo como tal exigiría un
# envoltorio tipo NSSM. La tarea consigue lo mismo —arranca sin sesión iniciada,
# sobrevive a reinicios y se reintenta sola si falla— sin añadir dependencias.
#
# Ejecutar como administrador:
#   .\instalar-servicio.ps1
#   .\instalar-servicio.ps1 -Quitar

[CmdletBinding()]
param(
    [switch]$Quitar,
    [int]$Puerto = 8787,
    [string]$Nombre = "agente-video"
)

$ErrorActionPreference = "Stop"

$esAdmin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()
).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $esAdmin) {
    throw "hace falta ejecutar esto como administrador: una tarea que arranca sin sesión iniciada no se puede registrar de otro modo."
}

if ($Quitar) {
    if (Get-ScheduledTask -TaskName $Nombre -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $Nombre -Confirm:$false
        Write-Host "tarea '$Nombre' eliminada. El panel dejará de arrancar solo."
    } else {
        Write-Host "no había ninguna tarea '$Nombre'."
    }
    exit 0
}

$Raiz = $PSScriptRoot
$Exe  = Join-Path $Raiz "agente-video.exe"
if (-not (Test-Path $Exe)) {
    throw "no se encontró $Exe. Compila primero: go build -o agente-video.exe .\cmd\agente-video"
}

# El directorio de trabajo importa: el agente busca perfiles\, bin\, assets\ y
# .env por ruta relativa, así que arrancar desde otro sitio lo dejaría sin nada.
$accion = New-ScheduledTaskAction -Execute $Exe `
    -Argument "servir -puerto $Puerto" -WorkingDirectory $Raiz

$disparador = New-ScheduledTaskTrigger -AtStartup

# SYSTEM permite que corra sin que nadie inicie sesión, que es justo el caso de
# un horario nocturno.
$identidad = New-ScheduledTaskPrincipal -UserId "SYSTEM" -RunLevel Highest

$opciones = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartInterval (New-TimeSpan -Minutes 2) -RestartCount 5 `
    -ExecutionTimeLimit ([TimeSpan]::Zero)   # sin límite: es un servicio, no un script

Register-ScheduledTask -TaskName $Nombre -Action $accion -Trigger $disparador `
    -Principal $identidad -Settings $opciones -Force | Out-Null

Start-ScheduledTask -TaskName $Nombre
Start-Sleep -Seconds 3

$t = Get-ScheduledTask -TaskName $Nombre
Write-Host ""
Write-Host "tarea '$Nombre' registrada y arrancada."
Write-Host "  estado : $($t.State)"
Write-Host "  panel  : http://127.0.0.1:$Puerto"
Write-Host ""
Write-Host "Arrancará sola con Windows, aunque nadie inicie sesión, y se"
Write-Host "reintentará hasta cinco veces si se cae."
Write-Host ""
Write-Host "Para quitarla:  .\instalar-servicio.ps1 -Quitar"
Write-Host ""
