# FAIR Visual Demo

This is a self-contained browser demo for the API noisy-neighbor scenario. It is designed for a 16:9 screen recording suitable for LinkedIn.

Open the demo directly:

```bash
open demo/fairness_visual/index.html
```

For recording, use the clean capture view:

```text
file:///Users/mihirsathe/Documents/GitHub/fair/demo/fairness_visual/index.html?recording=1
```

To render a deterministic MP4 without screen-recording permissions:

```bash
node demo/fairness_visual/render_video.mjs
```

Suggested capture settings:

- 16:9 viewport, preferably 1920x1080.
- 25-30 seconds.
- Start from the beginning and stop after the final banners appear.
- Use the headline: `Jain fairness: 0.218 -> 0.850 at 99.28% utilization`.

On macOS, position the browser window at the top-left of the display and record a 28-second clip with:

```bash
screencapture -v -V 28 -R0,0,1920,1080 demo/fairness_visual/fair-demo.mov
```

Convert it to a LinkedIn-friendly MP4:

```bash
ffmpeg -i demo/fairness_visual/fair-demo.mov -vf "scale=1920:-2" -pix_fmt yuv420p demo/fairness_visual/fair-demo.mp4
```

The animation uses the deterministic API noisy-neighbor results from `cmd/fairness_eval`:

- No FAIR: Jain `0.2184`, utilization `99.99%`.
- Tuned FAIR: Jain `0.8498`, utilization `99.28%`.
