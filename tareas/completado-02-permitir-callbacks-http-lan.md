# Tarea 02 — Permitir callbacks HTTP en LAN confiable

## Objetivo
Permitir que Manifest/OAuth usen URLs HTTP locales confiables como `http://dex.local:9090`, manteniendo HTTPS obligatorio para hosts públicos y webhooks.

## Alcance
- Aceptar loopback, IP privadas/link-local y nombres `.local`/`.home.arpa` cuando el caller habilita HTTP local.
- Mantener rechazo de HTTP para hosts públicos.
- Mantener `WebBaseURL`/`APIBaseURL` de GitHub con la política existente.
- Añadir pruebas y documentación.
- Publicar una versión patch para que Dex pueda consumirla.
