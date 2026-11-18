# SenseDeep ocean background

The login backdrop uses the real procedural WebGPU environment from
[Noniv/abyssal-garden](https://github.com/Noniv/abyssal-garden), pinned to
`a596354eff400d458bc2aeb9bdf5e0a9734b2bcf`.
Copyright (c) 2026 Maksymilian Dendura. MIT license; see `LICENSE`.

`source/` preserves the upstream snapshot. `../../scripts/abyssal-background.mjs`
adapts it into an isolated, self-hosted background page during Vite dev/build:

- Preserves the actual terrain, kelp geometry, water, caustics, shadows and volumetric light.
- Fixes the camera at the first upstream viewpoint; removes navigation, pointer lock, sound and menus.
- Keeps 25% of fish, 2 of 6 mantas, one third of bubbles, one quarter of marine snow and half the glints.
- Uses medium detail, a maximum of 1280 × 720 render pixels and a 24 fps cap, with adaptive resolution reduction.
- Removes the unused camera collision field; pauses rendering in hidden tabs and releases the GPU on teardown.
- Emits the original MIT license alongside the built background.

`OceanBackdrop.vue` uses WebGPU on capable desktop browsers at localhost/HTTPS.
Ordinary LAN HTTP cannot use WebGPU. It, mobile and smaller devices play the
self-hosted `public/backgrounds/abyssal-loop.webm` recording of **this same scene**.
The WebP poster appears during loading or when reduced motion / data saving is enabled.
No external asset host, API, audio or interactive 3D controls are required.

To regenerate the poster and recording after changing the scene, start Vite,
launch a separate Chrome instance with GPU enabled and a dedicated profile using
`--remote-debugging-port=9223`, then run from `frontend/`:

```sh
node scripts/capture-ocean.mjs http://127.0.0.1:5185 9223
```

This opens and closes only its own background tab. The output is committed as
static assets, so Docker and LAN clients do not need recording tools. The full
rendering source is included in the build; the asset recording is not a network download.
