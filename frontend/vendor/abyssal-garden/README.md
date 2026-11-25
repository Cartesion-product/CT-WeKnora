# SenseDeep ocean background

The login backdrop uses the real procedural WebGPU environment from
[Noniv/abyssal-garden](https://github.com/Noniv/abyssal-garden), pinned to
`a596354eff400d458bc2aeb9bdf5e0a9734b2bcf`.
Copyright (c) 2026 Maksymilian Dendura. MIT license; see `LICENSE`.

`source/` preserves the upstream snapshot. `../../scripts/abyssal-background.mjs`
adapts it into an isolated, self-hosted background page during Vite dev/build:

- Preserves the actual terrain, kelp geometry, water, caustics, shadows and volumetric light.
- Fixes the camera at the first upstream viewpoint; removes navigation, pointer lock, sound and menus.
- Keeps five fish from the school in front of the camera and one central manta, plus one third of bubbles, one quarter of marine snow and half the glints.
- Uses medium detail and a maximum of 1280 x 720 render pixels. The real-time preview has a 24 fps cap and adaptive resolution reduction.
- Removes the unused camera collision field; pauses rendering in hidden tabs and releases the GPU on teardown.
- Emits the original MIT license alongside the built background.

`OceanBackdrop.vue` plays the self-hosted 1280 x 720, 40-second, 30 fps
`public/backgrounds/abyssal-loop.webm` recording of **this same scene** on all
devices. The real-time WebGPU renderer remains available for regenerating the
recording, but no longer runs behind the login form.
The WebP poster appears during loading or when reduced motion / data saving is enabled.
No external asset host, API, audio or interactive 3D controls are required.

To regenerate the poster and recording after changing the scene, start Vite,
launch a separate Chrome instance with GPU enabled and a dedicated profile using
`--remote-debugging-port=9223`, install FFmpeg, then run from `frontend/`:

```sh
node scripts/capture-ocean.mjs http://127.0.0.1:5185 9223
```

The script renders each frame at a fixed scene time before encoding, so a slow
recording machine does not lower the playback frame rate. An optional third
argument specifies the FFmpeg executable path.

This opens and closes only its own background tab. The output is committed as
static assets, so Docker and LAN clients do not need recording tools. The full
rendering source is included in the build; the asset recording is not a network download.
