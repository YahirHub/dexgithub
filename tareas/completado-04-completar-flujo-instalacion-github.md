# Tarea 04 — Completar flujo de instalación GitHub App

## Objetivo
Permitir que el Manifest configure `setup_url`/`setup_on_update` para que GitHub pueda volver al panel después de instalar la App y elegir repositorios.

## Alcance
- Añadir `SetupURL` y `SetupOnUpdate` a `ManifestOptions`.
- Validar `SetupURL` con la misma política de callbacks locales/HTTPS.
- Serializar ambos campos sólo cuando exista `SetupURL`.
- Añadir pruebas de regresión.
- Publicar una versión patch para Dex.
