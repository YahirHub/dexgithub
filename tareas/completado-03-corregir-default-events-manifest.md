# Tarea 03 — Corregir default_events del GitHub App Manifest

## Objetivo
Evitar que un Manifest sin eventos explícitos serialice `default_events` como `null`; GitHub exige un arreglo JSON.

## Alcance
- Normalizar eventos ausentes a `[]`.
- Añadir prueba de regresión sobre el JSON generado.
- Validar y publicar un patch de DexGitHub.
- Actualizar Dex para consumir el patch y desplegarlo.
