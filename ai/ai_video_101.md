# Video Creation & Editing — Phase 1 Course (Beginner Edition)

> From Prompt to Picture Lock.
> Four lessons. Teaching mode is gentle and explains every term. Drills are harsh.
> Assumes you can run a command in a terminal and read JSON. **Zero** film or editing knowledge required.

---

## Prerequisites

You should be able to open a terminal, run a command, and read a JSON file. That's it. You do not need to have used Premiere, DaVinci Resolve, or Final Cut. You do not need a camera. You do not need to know what a codec is — that's section 1.5.

This course exists because AI video tools have made **generation** cheap and left everything else exactly as hard as it always was. The model hands you a five-second clip. It does not hand you a video. The gap between those two things is a real craft with a hundred years of accumulated vocabulary, and if you don't learn the vocabulary you cannot even *describe* what's wrong with your output, let alone fix it.

Teaching is in prose. Testing is in deliberately harsh drills. Do the drills.

---

## Before You Start: What "Making a Video" Actually Is

There are three jobs people collapse into the phrase "making a video." They are almost entirely different, and AI has only automated one of them.

**Production** is capturing or generating raw material: footage, stills, voice, music. This is the part AI video models do. You type a description, you get pixels. It used to require a camera, a location, a crew, and a day. Now it requires credits and thirty seconds.

**Post-production** is turning raw material into a thing a human will watch: choosing which pieces, in which order, for how long, with what sound underneath, with what text on top, exported in the right format for the right platform. AI has barely touched this. It is still you, a timeline, and a set of decisions.

**Direction** is deciding what the material should be *before* it exists: what the video is trying to accomplish, what the viewer feels at second 3 versus second 20, which shot carries which beat. AI has touched this least of all, because it's the part that requires knowing what you want.

The beginner's error is to think that better prompts are the whole job. They are not. They are the *production* half of one third of the job. A launch video that fails almost never fails because a clip looked bad. It fails because the first two seconds didn't earn the next five, or because there was no sound, or because the cuts landed on the wrong beats, or because it was 16:9 on a platform where 94% of viewers hold their phone vertically.

The mental shift you have to make: **you are not generating a video. You are generating raw material, and then you are building a video out of it.** The generator is one box in the middle of your pipeline. Everything around it — the shot list, the assembly, the sound, the export — is yours.

---

## A Small Glossary You'll See A Lot

I'll explain these properly as they come up, but bookmark this for quick reference:

- **Frame** = one still image. Video is a sequence of them shown fast enough to read as motion.
- **fps / frame rate** = frames per second. 24 = cinema, 30 = broadcast/web, 60 = sports/games/screen capture.
- **Resolution** = pixel dimensions, e.g. `1920×1080` ("1080p"), `3840×2160` ("4K UHD").
- **Aspect ratio** = shape of the frame. `16:9` horizontal, `9:16` vertical, `1:1` square, `4:5` feed-tall.
- **Codec** = the compression algorithm that encodes the pixels (H.264, H.265, ProRes, AV1).
- **Container** = the file wrapper holding video + audio + metadata (`.mp4`, `.mov`, `.mkv`). Not the same as the codec.
- **Bitrate** = data per second. More = better quality and bigger file.
- **Shot** = one continuous run of camera without a cut. The atomic unit of production.
- **Clip** = a piece of footage on your timeline. Usually a trimmed shot.
- **Cut** = the join between two clips. The atomic unit of editing.
- **B-roll** = supporting footage that isn't the main subject. Cutaways, product details, hands, screens.
- **Timeline / sequence** = the editor's workspace where clips are arranged in time on tracks.
- **Shot list** = the plan: every shot you need, described, before you generate any of them.
- **Text-to-video (t2v)** = generate a clip from a description alone.
- **Image-to-video (i2v)** = generate a clip that *starts from* an image you supply. Far more controllable.
- **Keyframe / first-last frame** = supplying the start (and sometimes end) image to pin a clip's endpoints.
- **Color grade** = deliberate adjustment of color and contrast for consistency and mood.
- **LUT** = a lookup table; a portable color transform you apply to footage.
- **LUFS** = the loudness unit platforms use to normalize audio. Your target, not peak dB.
- **ffmpeg** = the command-line tool that does essentially all video file manipulation. Your assembly layer.
- **Picture lock** = the point where edits to the cut stop and only sound/color work remains.

---

# Lesson 1: What Video Actually Is (Just Enough)

## 1.1 Why This Lesson Exists

In December 2012, Peter Jackson released *The Hobbit: An Unexpected Journey* in a format almost no one had seen in a cinema: 48 frames per second, double the century-old standard. Technically it was superior in every measurable way — sharper motion, less blur, less judder. Audiences hated it. The complaint, repeated in thousands of reviews, was that a $200 million fantasy epic looked like a cheap television soap opera or a behind-the-scenes video.

Nothing was broken. The lighting was the same, the sets were the same, the actors were the same. One number changed, and the entire emotional register of the image changed with it — because audiences had spent their whole lives learning, unconsciously, that 24fps motion blur means "cinema" and crisp 50/60fps motion means "live TV, news, home video, reality."

That's the lesson. **The technical parameters of video are not neutral containers for your content. They are part of the content.** Frame rate, aspect ratio, resolution, and codec all carry meaning to viewers who have never heard those words. If you don't understand them mechanically, you will produce videos that feel wrong for reasons you cannot name, and you will blame the model.

This lesson gives you the honest mechanical picture. Not the marketing version.

## 1.2 The One Trick: Still Images, Fast

Strip away everything and a video is:

> A sequence of still images, displayed in order at a fixed rate, optionally accompanied by a synchronized audio waveform.

That's it. There is no motion in a video file. There is a stack of pictures and a clock.

```
frame 0    frame 1    frame 2    frame 3    frame 4   ...
[ img ] -> [ img ] -> [ img ] -> [ img ] -> [ img ]
   |          |          |          |          |
   +----------+----------+----------+----------+
              displayed 24 times per second
   
   audio: ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~  (continuous, must stay in sync)
```

Your eye and brain fuse them into motion. Every property of video that follows — file size, sharpness, smoothness, how "expensive" it looks — is downstream of two questions: *how many pictures per second*, and *how much data per picture*.

Internalize this, because it explains a lot:

- **Why AI video is expensive**: a 5-second clip at 24fps is 120 full images the model must generate *consistently with each other*. Image generation is one picture. Video is 120 pictures that have to agree about physics.
- **Why AI clips are short**: consistency degrades as the stack gets longer. Faces drift, objects morph, physics breaks. Short clips are not a pricing decision; they're a fidelity ceiling.
- **Why editing is powerful**: a cut costs nothing and hides everything. Two good 4-second clips beat one mediocre 8-second clip, always.

## 1.3 Frame Rate: The Number That Sets the Feel

Frame rate is how many images per second. The standards are historical accidents that hardened into cultural meaning:

| fps | Where it comes from | What it reads as |
|---|---|---|
| **24** | The film industry standard since ~1927 (cheapest rate that carried optical sound) | Cinema. Story. Drama. |
| **25** | European mains electricity at 50Hz | Broadcast, Europe |
| **30** (really 29.97) | North American mains at 60Hz, plus a color-TV fudge | Web, broadcast, "normal video" |
| **60** | Twice broadcast; what screens and games run at | Sports, gameplay, screen recordings, hyper-real |

Two mechanical consequences you must design around:

**Motion blur.** A real camera's shutter is open for a fraction of each frame, and anything moving during that window smears across the image. The convention is the **180-degree shutter rule**: shutter speed ≈ 1 / (2 × fps). At 24fps that's a 1/48s exposure. That blur is what makes 24fps read as smooth rather than as a slideshow — and it's a large part of what "cinematic" means. Higher frame rates give each frame less blur, which is why 48fps looked clinical and video-ish to *Hobbit* audiences.

**Mixing rates causes judder.** 24 does not divide evenly into 30. Putting 24fps material into a 30fps timeline requires duplicating some frames on a repeating pattern (historically "3:2 pulldown"), which produces a subtle stutter on smooth camera moves. Mixing rates in one project is the single most common source of "why does this look slightly broken and I can't tell why."

**The 101 rule: pick one frame rate for the whole project before you generate a single clip, and force everything to it.** For social and web, 30fps is the safe default; for anything meant to feel like film, 24. Whichever you choose, put it in your config and never mix.

## 1.4 Resolution, Aspect Ratio, and Why Vertical Is Not a Crop

**Resolution** is the pixel grid: `1920×1080` (1080p / "Full HD"), `3840×2160` (4K UHD), `1080×1920` (vertical 1080p). More pixels means more detail and more data — and, in AI generation, more credits and more time.

**Aspect ratio** is the *shape*, and it matters more than resolution for whether your video works:

```
16:9  [==============]     horizontal. YouTube, web embeds, TV.
 1:1  [======]             square. Feed posts.
 4:5  [=====]              tall-ish. Optimized for mobile feed real estate.
      [     ]
 9:16 [===]                full vertical. Stories, Reels, Shorts, TikTok.
      [   ]
      [   ]
```

Here's the part beginners get wrong. **Aspect ratio is a composition decision, not an export setting.** A shot composed for 16:9 — subject at one third, negative space to the right, product entering frame left — becomes garbage when you crop it to 9:16, because the crop throws away 60% of the width including the thing the composition was built around.

If you need both a horizontal and a vertical cut (and for a launch, you usually do), you have three honest options:

1. **Generate twice**, once at each aspect ratio, from the same shot description. Costs double. Best result.
2. **Generate at the widest ratio with vertical in mind** — deliberately compose subjects center-frame so a center crop survives. Cheapest. Compromised on both.
3. **Generate vertical as primary** and letterbox/pad it into 16:9 with a background. Fine for social-first campaigns, weak as a website hero.

Also learn **safe areas**: on vertical platforms, roughly the top 10% and bottom 20% of the frame are covered by the app's own UI — captions, usernames, buttons, the "follow" chrome. Any text you put there is invisible to a real viewer. Compose important content into the middle band.

## 1.5 Codec, Container, Bitrate — Three Different Things

Beginners treat "MP4" as a quality setting. It isn't; it's a box.

- **Codec** is the compression algorithm — how pixels become bytes. Common ones: **H.264** (universal, plays everywhere, the safe delivery choice), **H.265/HEVC** (about half the size at the same quality, less universal), **AV1** (newer, royalty-free, increasingly used by streaming platforms), **ProRes / DNxHR** ("intermediate" codecs — huge files, minimal quality loss, used *while editing*, not for delivery).
- **Container** is the file wrapper that holds the encoded video, the audio, and metadata: `.mp4`, `.mov`, `.mkv`, `.webm`. The same H.264 video can live in an `.mp4` or a `.mov`. The extension tells you the box, not the contents.
- **Bitrate** is data per second — the actual quality dial. Higher bitrate means fewer compression artifacts (blockiness in gradients, mush in fast motion) and bigger files.

Two mechanisms you must know:

**Lossy compression is a one-way door.** H.264 and H.265 throw away information permanently. Every time you decode and re-encode a lossy file, you lose a little more. This is **generation loss**. Export → import → export → import, four times, and your gradients are banded and your text edges are crunchy. The fix is discipline: do all intermediate steps in a high-bitrate or intermediate codec, and encode to a lossy delivery codec exactly **once**, at the end.

**Chroma subsampling.** Most delivery video stores color at *half* the resolution of brightness (written `4:2:0`), because eyes are much more sensitive to luminance than to color detail. Usually invisible. Very visible on saturated fine text, thin colored graphics, and green-screen edges — which is exactly the content in a product launch video with an overlaid logo. If your brand-colored text looks smeared, subsampling is a likely culprit; keep text overlays as a final composite step at high bitrate rather than baking them into an already-compressed clip.

**The 101 rule: intermediate = high quality and forgiving; delivery = H.264 in an MP4, encoded once.**

## 1.6 Color Is a Pipeline, Not a Vibe

Every clip you generate will have slightly different color: different white balance, different contrast, different black level. Cut them together untouched and the result reads as amateur — not because any single shot is bad, but because the *jumps* between them announce that these things were made separately.

The vocabulary you need:

- **Color space / gamma** — the agreed mapping from stored numbers to displayed light. For ordinary HD/SDR video that's **Rec.709**. HDR uses different, larger spaces (Rec.2020, PQ). Mismatching them is why exported video sometimes looks washed out or crushed compared to what you saw in the editor.
- **Correction vs grading** — *correction* makes shots match each other and look neutral (fixing exposure and white balance). *Grading* then applies a deliberate look on top (teal shadows, warm highlights, lifted blacks). Correct first, grade second, in that order, always.
- **LUT** — a lookup table that maps every input color to an output color. A portable "look" you can apply consistently across every shot in a project.

For AI-generated material specifically: your leverage is mostly in the *prompt* (naming a consistent lighting setup and palette across every shot) and in a *single grade applied to the finished assembly* rather than per-clip fiddling. One LUT over the whole timeline unifies mismatched generations better than any amount of per-shot correction.

## 1.7 Audio Is Half the Video and Almost All of the Perceived Quality

This is the most under-taught thing in beginner video, and the one that most reliably separates "looks like a real launch" from "looks like AI slop."

Viewers forgive a soft image. They do not forgive bad sound. Muddy voice, room echo, a music bed that fights the voice, or an abrupt silence where a sound should be — any of these read instantly as *cheap*, even to people who couldn't tell you why.

The mechanics:

- **Sample rate**: video audio is 48 kHz by convention (music is 44.1 kHz). Mixing them causes drift and resampling artifacts. Pick 48 kHz.
- **Levels**: the meter that matters for delivery is **LUFS** (loudness units, full scale) — an average perceptual loudness, not a peak. Every major platform *normalizes* uploads toward its own target, so mastering louder than the target gains you nothing and just costs you dynamic range. Targets differ by platform and change over time; check the current spec for where you're publishing rather than trusting a number you read once.
- **True peak** should stay below 0 dBFS with headroom (around −1 dBFS) so lossy encoding doesn't introduce clipping.
- **Sync**: audio and video are separate streams that must agree about time. Lip sync errors of even ~50ms are perceptible; the eye is very good at catching a mouth that moves before the sound.
- **Ducking**: automatically lowering the music whenever the voice speaks. This is why professional videos have music you *feel* but never struggle to hear over. Doing this by hand is the single highest-return editing move for a talking video.

Three layers make up almost every finished video's soundtrack: **voice** (narration or dialogue), **music** (the bed, carrying pace and emotion), and **SFX** (whooshes on transitions, UI clicks, ambience). A launch video with only music sounds like a template. One with a whoosh on each transition and a click when the UI element appears sounds designed.

## 1.8 What AI Video Does *Not* Give You

A list to tape to your monitor. Out of the box, from a generation model, you get **one short clip that matches a description**. You do not get:

- **A cut.** Ordering, trimming, and pacing are yours.
- **Continuity.** The model has no memory of your previous clip. Same character, same product, same lighting across shots is a problem *you* solve with reference images and identity models.
- **Reliable text.** Generated text in-frame is frequently mangled and will be off-brand even when legible. Real titles are composited in post, in your actual typeface.
- **Sync.** Voice, music, and picture landing on the same beats is an editing job.
- **Sound design.** Even models that produce audio give you ambience, not a designed mix.
- **A specific duration.** You asked for 30 seconds of story; you got clips. Fitting them to 30 seconds is editing.
- **Determinism.** The same prompt twice gives you two different clips. Plan for re-rolls, and plan for the budget they consume.
- **Physical accuracy about your product.** The model has never seen your device. If the product's actual shape matters, the product must enter the pipeline as a real image, not as a description.

Every one of these gaps is a job for you. Most of this course is the discipline of filling them.

## 1.9 Summary: The Rules

1. **Video is stills plus a clock.** Everything else is downstream of pictures-per-second and data-per-picture.
2. **Frame rate carries meaning.** 24 reads as cinema, 30 as web, 60 as live/real. Pick one per project and never mix — mixing causes judder.
3. **Aspect ratio is composition, not an export setting.** Cropping 16:9 to 9:16 destroys the composition. Decide the primary ratio first; respect platform safe areas.
4. **Codec ≠ container ≠ bitrate.** Lossy re-encoding is a one-way door; encode to delivery exactly once.
5. **Color is a pipeline**: correct first, grade second, one look over the whole timeline.
6. **Audio is half the video and most of the perceived quality.** 48 kHz, platform-appropriate loudness, headroom, sync, and ducking.
7. **The model gives you clips, not a video.** Cuts, continuity, text, sync, sound, and duration are all yours.

## 1.10 Drill 1

Rules: show mechanism, not vibes. "It looks more cinematic" gets zero credit unless you can say *what changes mechanically*. Write your answers down.

**Q1. Mechanism.**
Explain why the *same scene, same lighting, same actors* can read as "expensive film" at one frame rate and "cheap TV" at another. Your answer must mention shutter angle, motion blur, and learned viewer association, and must not use the word "cinematic" as an explanation of itself. At least 150 words.

**Q2. The aspect ratio trap.**
You generate a beautiful 16:9 hero shot: the product sits in the right third of frame, a person's hands enter from the left, deep negative space above. Your marketing lead asks for a 9:16 version "just crop it." List four specific things that break, and then describe the two different ways you could have planned the shot so both ratios were possible. What does each cost you?

**Q3. Generation loss.**
Trace a file through this pipeline and say where quality is lost and how much, at each step: model outputs H.264 MP4 → you trim it in an editor and export H.264 → you add captions in another tool and export H.264 → you upload, and the platform re-encodes. Then redesign the pipeline to lose the minimum, and state the one place a lossy encode is unavoidable.

**Q4. Audio triage.**
You have a 40-second launch video with a voiceover, a music bed, and no other sound. Viewers say it "feels amateur" but can't say why. List five specific, distinct audio-level causes, and for each name the fix in one sentence. At least two must involve the *relationship* between tracks rather than either track alone.

**Q5. Draw the boundary.**
For each, say whether a bare AI video model gives it to you out of the box (yes/no) and, if no, name the technique or tool from this course that provides it:
(a) The same character's face in shot 1 and shot 7.
(b) A clip that is exactly 4.0 seconds long.
(c) Your product's actual logo, correctly spelled, on screen.
(d) A music track that swells when the product appears.
(e) A camera that pushes in slowly.
(f) A 9:16 and a 16:9 version of the same idea.

**Q6. Reading / doing.**
Install `ffmpeg`. Run `ffprobe` on any video file you have and read the output. Then answer:
- What codec, container, resolution, frame rate, and bitrate does the file use, and which line told you each?
- What are the audio stream's sample rate and channel count?
- If you had to hand this file to someone who only accepts 1080p H.264 MP4 at 30fps, which properties would need to change?

---

# Lesson 2: The Grammar of a Shot (Directing, Not Wishing)

## 2.1 Why This Lesson Exists

Here is the prompt beginners write:

```
a cool video of my app being used, modern and professional
```

And here is why it fails. Every word in it is a *judgment* ("cool", "modern", "professional") rather than a *specification*. The model resolves judgments to whatever is statistically most common for those words, which is by definition the most generic possible output. You have asked for the average of everything and received it.

Now here is the same intent, written by someone who knows the vocabulary:

```
Medium close-up, 50mm, shallow depth of field. A woman's hands hold
a phone showing a dashboard; she taps once and looks up. Soft key light
from window left, cool ambient fill, dark neutral background falling off
to black. Camera slowly pushes in. Calm, controlled, no camera shake.
```

Same idea. But now every property that determines the image has been named: shot size, lens, focus, subject, action, light direction and quality, background, camera movement, and energy. The model is no longer guessing.

This lesson teaches the vocabulary. It is a hundred years old, it is shared by every cinematographer alive, and — crucially — **it is what the models were trained on.** Film language works on these tools because the training data is full of film. Marketing adjectives do not work, because "professional" describes no pixels.

## 2.2 Shot Size: How Much of the Subject Is in Frame

The first decision, and the one that carries the most meaning:

```
EWS  Extreme Wide  - subject tiny in environment. Establishes place, scale.
WS   Wide          - full body, environment visible. Context and action.
MS   Medium        - waist up. The conversational default.
MCU  Medium Close  - chest up. Intimate but not intense.
CU   Close-Up      - face fills frame. Emotion. Detail.
ECU  Extreme Close - eye, hand, a switch, a texture. Intensity, specificity.
```

The rule that makes this useful: **shot size is emotional distance.** Wide means "observe this." Close means "feel this." A launch video that stays in medium shots the whole way through feels flat not because any shot is wrong but because the emotional distance never changes.

For products specifically: **ECU sells texture and quality** (the stitch, the anodized edge, the pixel-crisp UI), **MS sells use** (a person actually doing the thing), **WS sells context** (where this fits in a life). A good 30-second launch cycles through all three.

## 2.3 Lens and Depth of Field

Focal length, measured in millimeters, controls two things at once:

- **Field of view** — how much fits in frame. Low numbers (16–24mm) = wide. High numbers (85–200mm) = narrow/telephoto.
- **Perspective compression** — the apparent depth between foreground and background. Wide lenses *stretch* space and exaggerate anything close to the camera (this is why 16mm makes rooms look cavernous and noses look large). Long lenses *flatten* space, stacking background against subject.

Rough working vocabulary:

| Focal length | Reads as | Use for |
|---|---|---|
| 16–24mm | Immersive, distorted, energetic | Environments, FPV/drone energy, "inside the action" |
| 35mm | Natural but roomy | Documentary, lifestyle, hands-and-product |
| 50mm | Closest to human vision, neutral | The safe default for people |
| 85mm | Flattering, isolating | Portraits, hero shots of a person |
| 100mm+ macro | Extreme detail, razor-thin focus | Product texture, ECU |

**Depth of field** is how much of the scene is in sharp focus. Shallow (a blurred background, "bokeh") isolates the subject and reads as expensive, because it implies a large sensor and a fast lens. Deep (everything sharp) reads as informational, documentary, or phone-camera. Naming this explicitly — "shallow depth of field, background falls out of focus" — is one of the highest-value phrases you can put in a prompt.

## 2.4 Lighting: Direction, Quality, Ratio

Lighting is the single biggest determinant of whether an image reads as expensive, and it decomposes into three independent choices:

**Direction** — where the light comes from relative to the camera.
- *Front*: flat, safe, boring. Passport photos.
- *Side (45°)*: reveals texture and shape. The workhorse.
- *Back / rim*: subject edge glows, separates from background. Drama, premium.
- *Top / bottom*: interrogation, horror, or "the product is on a lit pedestal."

**Quality** — hard or soft.
- *Hard* light (small source: bare bulb, direct sun) = sharp-edged shadows, high contrast, tension, drama.
- *Soft* light (large source: window, overcast sky, big diffuser) = gradual shadow edges, flattering, calm, premium consumer.

**Ratio** — how bright the fill side is relative to the key side. Low ratio (fill close to key) = bright, upbeat, commercial. High ratio (deep shadows) = moody, cinematic, serious.

The standard three-point setup, which you can name directly in prompts:

```
        [ back / rim light ]      <- separates subject from background
                 |
                 v
    [key] ---> SUBJECT <--- [fill]
   (main,        |          (soft, lifts
   directional)  v          the shadow side)
              CAMERA
```

Also learn the **named natural conditions**, because they're compact and models understand them well: *golden hour* (low warm sun, long shadows), *blue hour* (post-sunset ambient, cool and soft), *overcast* (giant soft box, no shadows), *hard noon* (harsh top light, usually avoid), *practical lighting* (light sources visible in shot — lamps, screens, neon).

## 2.5 Camera Movement: One Per Shot

Movement types, and what each *means*:

| Move | Mechanically | Reads as |
|---|---|---|
| **Static / locked off** | No movement | Stability, confidence, focus |
| **Pan** | Camera rotates horizontally, fixed position | Surveying, revealing |
| **Tilt** | Camera rotates vertically | Revealing scale, up or down |
| **Push in / dolly in** | Whole camera moves toward subject | Growing intensity, "pay attention" |
| **Pull out / dolly back** | Camera retreats | Release, reveal of context, ending |
| **Truck / track** | Camera moves laterally alongside | Following, momentum |
| **Crane / boom** | Camera rises or falls | Scale, arrival, triumph |
| **Orbit / arc** | Camera circles the subject | Showcase, "look at this object" |
| **Handheld** | Small irregular shake | Urgency, realism, documentary |
| **Zoom** | Lens focal length changes, camera stationary | Cheap, televisual, or deliberately retro |

Note the last row. **A zoom is not a push-in.** A push-in physically changes the viewer's position in space, so the relationship between foreground and background shifts. A zoom just magnifies. Zooms read as cheaper for exactly this reason, and models will happily give you either if you name it.

**The one rule that matters most for AI video: one dominant camera move per clip.** Every generation model degrades when asked to do multiple movements in one shot ("pan left then push in as it rises") — you get warping, morphing geometry, and a subject that changes identity mid-move. If your idea needs two moves, that's two shots and a cut. It will look better anyway.

## 2.6 The Shot Prompt Formula

Put 2.2–2.5 together in a fixed order. Order matters because these models weight early tokens more heavily, so the most important thing goes first.

```
[shot size] + [subject, concretely described] + [what they do]
+ [ONE camera movement] + [lighting: direction, quality, condition]
+ [lens / depth of field] + [palette and mood] + [what NOT to do]
```

Worked example:

```
Extreme close-up of a matte black speaker's woven fabric grille, water
beading on the surface. A single drop rolls down and off the edge.
Camera slowly orbits 20 degrees left. Hard rim light from behind right,
cool blue, deep black background. 100mm macro, razor-thin depth of field.
Clinical, premium, restrained. No people, no text, no camera shake.
```

Four things that example does deliberately:

1. **The subject is a noun with properties**, not a category. "Matte black woven fabric grille," not "a speaker."
2. **The action is a single physical event.** One drop, one roll. Not "water splashing dramatically everywhere," which is a request for morphing chaos.
3. **One move**, with a magnitude. "Orbits 20 degrees" is a spec; "orbits" is a wish.
4. **Negative constraints** at the end. Models generate text badly and add people uninvited; saying "no text, no people" is cheap insurance.

**Set duration, aspect ratio, resolution, and model as parameters, not as prose.** Writing "a 5-second vertical video" inside the prompt wastes tokens on something that is a flag (`--duration 5 --aspect_ratio 9:16`) and sometimes actively confuses the generation.

## 2.7 Continuity: The Rules That Make Shots Belong Together

Individual shots are easy. Making six shots feel like one video is where it gets hard, and there are formal rules for it.

**The 180-degree rule (screen direction).** Draw an imaginary line through your subject/action. Keep the camera on one side of it for all shots in a scene. Cross it and a character who was facing right suddenly faces left, and the viewer — who cannot articulate why — feels disoriented. In practice for AI video: decide "the product faces camera-right" or "she walks left-to-right" and specify it in *every* shot prompt of that sequence.

**The 30-degree rule.** Consecutive shots of the same subject should differ by at least 30 degrees of angle or a full shot size, or the cut reads as a glitch (a "jump cut") rather than a cut.

**Eyeline and direction of travel.** If shot A shows someone looking off-frame right, shot B should show what's on their right. If a product slides out of frame right, it should enter the next shot from frame left.

**Lighting continuity.** This is where AI video most often collapses. Six shots generated independently will have six different key light directions and color temperatures. The fix is mechanical: **write the lighting clause once and paste the identical text into every shot prompt in the sequence.** Same words, same lighting.

**Subject continuity.** The model has no memory. Same face across shots requires an identity/reference mechanism (a trained character reference, or supplying the same reference image). Same product across shots requires supplying the actual product image as input. Descriptions alone will drift, guaranteed.

## 2.8 Image-to-Video Is the Real Technique

This is the most practically important paragraph in the lesson.

**Text-to-video gives the model two hard jobs at once**: invent the entire look of the scene, *and* animate it coherently. It will do both mediocrely, and you cannot iterate cheaply because every re-roll changes everything.

**Image-to-video splits the problem.** You generate (or shoot, or already own) a still image and get it exactly right — composition, lighting, product accuracy, brand color — at image prices, which are a small fraction of video prices. Then you hand that image to the video model and ask only for *motion*.

```
   TEXT-TO-VIDEO                   IMAGE-TO-VIDEO
   -------------                   --------------
   prompt                          prompt -> still image  (cheap, iterate freely)
     |                                          |  approve the look
     v                                          v
   model invents look              still + short motion prompt
   AND motion at once                          |
     |                                          v
     v                             model only has to animate
   one expensive dice roll         (controlled, repeatable)
```

Consequences:

- **Iterate on stills, not on clips.** Ten image variants cost a fraction of ten video re-rolls.
- **Motion prompts get short.** Once the image is fixed, the video prompt is one sentence about movement — "camera slowly pushes in, subtle ambient motion in the fabric." Long descriptive prompts on an i2v job fight the input image and cause drift.
- **First-and-last-frame control**, where a model supports it, is stronger still: supply both endpoints and the model interpolates. This is how you get a shot that *ends* exactly where the next shot needs to begin.
- **Chain shots by extracting frames.** Take the final frame of clip N and use it as the start image of clip N+1, and the two clips become a continuous move you can cut invisibly.

For anything involving your real product, image-to-video isn't a preference — it's the only way the product will look like itself.

## 2.9 Summary: The Rules

1. **Specify, don't judge.** "Professional" describes no pixels. Shot size, lens, light direction, and movement do.
2. **Shot size is emotional distance.** Vary it. ECU sells texture, MS sells use, WS sells context.
3. **Lens sets perspective, not just framing.** Shallow depth of field is the cheapest "expensive" signal there is.
4. **Lighting decomposes into direction, quality, and ratio.** Name all three, or name a natural condition that implies them.
5. **One dominant camera move per clip.** Two moves is two shots and a cut, and it looks better.
6. **Use the formula, in order**, and put duration/ratio/model in flags, not prose. Add negative constraints.
7. **Continuity is engineered, not hoped for**: identical lighting clause in every prompt, consistent screen direction, reference images for identity and product.
8. **Image-to-video is the real technique.** Iterate on cheap stills; ask the video model only for motion.

## 2.10 Drill 2

Rules: show the prompt and the reasoning. "Add 'cinematic, 8k, masterpiece'" gets zero credit — those are not specifications and you must explain why. Write your answers down.

**Q1. Rewrite for production.**
Here's a prompt that "worked in the demo":

```
a cool shot of our app on a phone, modern and clean
```

Rewrite it using the 2.6 formula. Every clause must be present. Then list the four decisions the original left to the model, and for each state what generic default the model would most likely have picked and why that's bad.

**Q2. Build a sequence.**
Design a six-shot sequence for a 25-second launch video for a physical product of your choosing. For each shot give: shot size, lens, camera move, and one line of action. Then state which single lighting clause appears identically in all six, and identify the two shots where you'd break the pattern deliberately and why.

**Q3. Continuity red-team.**
You generate six shots independently and cut them together. List six distinct continuity failures you should expect, ranked by how likely they are. For each, name the specific preventive measure at the prompt or pipeline level — not "regenerate it."

**Q4. Move selection.**
For each beat, choose one camera move and justify in one sentence using 2.5:
(a) The opening two seconds that must stop a thumb from scrolling.
(b) The moment the product's key feature is revealed.
(c) A shot establishing the environment the product lives in.
(d) The final shot under the call to action.
Then: explain mechanically why "slow push in while panning right and craning up" is a bad instruction to a generation model, in terms of what the model has to keep consistent.

**Q5. t2v vs i2v.**
For each, choose text-to-video or image-to-video and justify, including the cost argument:
(a) An abstract background loop of drifting particles behind your title card.
(b) A hero shot of your actual product, which has a distinctive shape and a logo.
(c) A crowd scene establishing "people everywhere use this."
(d) A shot that must end on exactly the frame where the next shot begins.
For (b), describe the full sequence of steps from "I have a product photo" to "I have a 4-second clip."

**Q6. Reading / doing.**
Watch any 30–60 second product launch video from a company whose work you admire, with the sound off, and write a shot list of it: every shot, its size, its move, and its duration in seconds. Then watch it again with sound and mark where music changes and where sound effects land.
- How many shots are there, and what's the average shot length?
- How many distinct camera moves are used, and how many shots are locked off?
- Where does the shot size change most sharply, and what beat does that change land on?

---

# Lesson 3: Editing — Where the Video Is Actually Made

## 3.1 Why This Lesson Exists

Around 1918, the Soviet filmmaker Lev Kuleshov ran an experiment. He took a single, unchanging close-up of the actor Ivan Mosjoukine's face — one shot, one expression, no performance at all — and intercut it three times: once with a bowl of soup, once with a girl in a coffin, once with a woman on a divan. Audiences praised the actor's range. They saw hunger in the first, grief in the second, desire in the third. They were looking at the *identical frames* every time.

The meaning was not in the shot. The meaning was in the **juxtaposition** — a thing that exists nowhere in the raw material and is created entirely at the cut.

This is the **Kuleshov effect**, and it is the reason editing is not "assembly." A cut is an authorial act. The same six clips, in two different orders, with two different rhythms, are two different videos with two different arguments. If you think of your AI clips as the product and the edit as packaging, you will make bad videos with excellent clips — which is, currently, the single most common failure mode in AI-generated marketing content.

The clips are the vocabulary. The edit is the sentence.

## 3.2 The Timeline Model

Every editor — Premiere, Resolve, Final Cut, CapCut, or a script you write yourself — is a view onto the same data structure:

```
        0s        2s        4s        6s        8s       10s
        |---------|---------|---------|---------|---------|
V2      |            [ logo overlay ]        [ CTA card ]     <- graphics
V1  [ shot A ][ shot B ][   shot C   ][ shot D ][ shot E ]    <- picture
A1  [~~~~~~~~ voiceover ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~]     <- voice
A2  [~~~~~~~~~~~~~~ music bed ~~~~~~~~~~~~~~~~~~~~~~~~~~]     <- music
A3       [whoosh]      [click]         [whoosh]               <- SFX
```

The concepts:

- **Track** — a horizontal lane. Higher video tracks composite *over* lower ones. Audio tracks sum together.
- **Clip** — a reference to a source file plus an **in point** and **out point** (which portion you're using). Trimming does not modify the source file; it changes the in/out. This is called **non-destructive** editing and it's why you can always change your mind.
- **Ripple** — a trim that shifts everything after it, changing total duration.
- **Roll** — a trim that moves the boundary between two adjacent clips, keeping total duration fixed.
- **Insert vs overwrite** — inserting pushes existing clips later; overwriting stamps on top of them.

If you're building this in code rather than an app, the equivalent structure is a **manifest**: a JSON file listing each shot's source file, in point, out point, track, and start time. Your assembly script reads it and calls `ffmpeg`. Same model, no GUI. We'll build one in Lesson 4.

## 3.3 Cuts: The Vocabulary of Joins

| Join | What it is | Use it for |
|---|---|---|
| **Hard cut** | Instant switch, frame to frame | ~95% of all cuts. The default. |
| **Cut on action** | Cut mid-movement, action continues across the join | Making cuts invisible |
| **Match cut** | Shape, motion, or composition rhymes across the cut | Elegant transitions between ideas |
| **Cutaway** | Brief shot of something else, then back | Hiding a jump, adding detail, fixing pacing |
| **Jump cut** | Same subject, near-identical framing, time removed | Deliberate energy (vlogs), or an accident |
| **Cross dissolve** | One shot fades into another | Passage of time, softness. Overused by beginners. |
| **J cut** | Audio of the *next* shot starts *before* its picture | Smoothness; the ear leads the eye |
| **L cut** | Audio of the *current* shot continues *under* the next picture | Continuity; conversation flow |

Two things to internalize.

**Cut on action.** If a hand reaches for a phone in shot A and the phone is picked up in shot B, cutting in the *middle* of the reach makes the join nearly invisible — the viewer's attention is on the motion, not the edit. Cutting after the reach completes and before the pickup starts makes the join obvious and dead. Same clips, different cut point, completely different feel. This is the highest-value editing habit you can build.

**J and L cuts are why professional edits feel smooth.** Beginners cut picture and sound at the same instant, every time, which produces a hard chop the viewer subconsciously registers on every cut. Offsetting audio by even a few frames — letting the next scene's ambience or the next line's first syllable arrive early — glues shots together. In a manifest-driven pipeline this is just: audio in-point ≠ video in-point.

**On dissolves and fancy transitions**: the reason professionals use hard cuts almost exclusively is that a transition *is an announcement that a transition is happening*. Every spin, glitch, and zoom-blur transition draws attention to the edit rather than the content. Use them when the announcement is the point (a deliberate scene break, a stylistic beat drop) and hard-cut everything else.

## 3.4 Pacing, Rhythm, and the First Two Seconds

**Shot length is your pacing dial.** Average shot length under ~1.5s reads as urgent, energetic, hype. Around 3–4s reads as considered and premium. Over ~6s reads as slow, confident, or boring depending on whether anything is happening. A launch video that holds one pace throughout is monotonous regardless of which pace it picks; the standard move is to open fast, slow down for the substance, and speed up into the call to action.

**Cut to the music.** Music has a beat grid, and cuts that land on the beat feel intentional while cuts that land between beats feel sloppy — even to viewers with no musical training. Find the beat positions once (in seconds), snap your cut points to them, and the whole thing tightens up. This alone will do more for perceived quality than another round of clip re-rolls.

**The first two seconds decide everything on social.** On a scrolling feed you are competing against a thumb. The mechanics of that competition:

- **Start at the most interesting frame.** Not a logo. Not a fade from black. Not an establishing shot. The single most arresting image you have, on frame one.
- **Movement in frame one** stops the scroll better than a static image, even a beautiful one.
- **Assume no sound.** A large share of feed viewing starts muted, so the first two seconds must work silently — which means text or a self-evident visual.
- **Front-load the promise.** What the viewer gets by staying should be legible almost immediately.

A useful structure for a 30-second launch:

```
0:00-0:02   HOOK       the arresting image + the promise, works muted
0:02-0:08   PROBLEM    the tension your product resolves
0:08-0:20   PRODUCT    what it is, in use, with the one key feature clear
0:20-0:26   PROOF      the reason to believe (result, number, testimonial)
0:26-0:30   CTA        exactly one action, on screen long enough to read
```

Every second of that is a decision about clip choice, order, and duration. None of it is a prompt.

## 3.5 Text, Captions, and Titles

- **Burn in captions.** Most feed viewing starts muted; a video whose meaning depends on unheard audio is a video most people bounce from. Generate a transcript, produce an `.srt`, and burn it into the picture (or use the platform's caption feature — but burned-in survives every re-share).
- **Never rely on model-generated text in-frame.** It will be misspelled, off-brand, and unfixable. Composite real text over the picture in your real typeface, in post, where it's editable and pin-sharp.
- **Hold long enough to read.** The floor is roughly one second plus a fraction of a second per word; a URL or a price needs longer. Beginners consistently cut titles a beat too early because *they* already know what it says.
- **Respect safe areas** (1.4). Bottom fifth of a vertical video belongs to the platform's UI, not to you.
- **Legibility beats brand.** High-contrast text with a subtle shadow, scrim, or plate behind it. Thin light-weight type over a busy moving background is unreadable, no matter what the brand guidelines say.

## 3.6 The Sound Mix

Three layers, mixed in this order of priority:

1. **Voice is king.** Everything else exists to support it. If a choice makes the voice less clear, it's the wrong choice.
2. **Music carries pace and emotion.** Choose it *before* you edit picture, because your cut points will follow its beats. Changing the track after the edit means re-editing.
3. **SFX sell the visuals.** A whoosh on a transition, a click when a UI element lands, a low sub hit on the logo reveal. These are the difference between "a template" and "designed."

The two techniques that matter most:

- **Ducking**: automatically drop the music several dB whenever the voice is present, and bring it back in the gaps. Nearly every professional video does this and almost no beginner does.
- **Silence is a tool.** Cutting the music to nothing for half a second before the product reveal makes the reveal land. Continuous wall-to-wall music flattens every beat equally.

And the boring but essential one: **check your licensing.** Music, stock footage, fonts, and voice likenesses all carry rights. "It was in a library" is not a license, and platform-level copyright detection is automated and unforgiving. For anything commercial, confirm the terms of every asset — including what the AI tool's own terms say about commercial use of what it generates on your plan.

## 3.7 Deliverables: One Edit, Many Exports

A launch is never one file. Plan the matrix before you edit:

```
                16:9        9:16        1:1
  30s full      web hero    -           -
  15s cut       pre-roll    Reels       feed
  6s bumper     -           story       -
  still frames  thumbnail   cover       cover
```

Two rules that save enormous rework:

- **Cut the longest version first**, then derive shorter ones by removing beats. Deriving long from short is impossible; deriving short from long is trimming.
- **Reframe deliberately, don't auto-crop.** Automatic reframing tools are useful starting points and reliably wrong on your hero shot. Check every shot of every ratio by eye.

Also: **export a master.** One high-quality, minimally-compressed file that everything else is derived from (1.5). When someone asks for a different length or ratio in three months, you re-derive from the master rather than re-compressing a compressed delivery file.

## 3.8 Summary: The Rules

1. **Meaning is made at the cut**, not in the clip. Same material, different order, different video.
2. **The timeline is tracks, clips, and in/out points.** In code, that's a manifest. Editing is non-destructive.
3. **Hard cut is the default.** Cut on action to make joins invisible; J and L cuts to make the whole thing feel smooth. Fancy transitions announce themselves.
4. **Shot length is pacing.** Vary it across the video; snap cuts to the music's beat grid.
5. **The first two seconds must work muted**, start on the most arresting frame, and front-load the promise.
6. **Burn in captions; composite real text in post.** Model-generated on-screen text is not usable.
7. **Voice > music > SFX.** Duck the music under the voice. Use silence deliberately.
8. **Cut longest first, derive the rest, keep a master.** Verify every reframe by eye. Check every license.

## 3.9 Drill 3

Rules: show the timeline and the reasoning. "I'd make it more dynamic" gets zero credit. Write your answers down.

**Q1. Kuleshov in practice.**
You have three clips: (a) a person looking at their laptop, expression neutral; (b) a dashboard showing a number going up; (c) a dashboard showing an error state. Describe the two different videos you can build from the same three clips, what each one asserts, and exactly which ordering choice creates the assertion. Then explain why no amount of re-prompting clip (a) could produce either meaning on its own.

**Q2. Fix the pacing.**
A 30-second launch video has 6 shots of exactly 5 seconds each, hard cuts, a continuous music bed, no SFX, and text that appears at 0:26. List six specific structural problems and give the fix for each. At least two fixes must change the *order* of material, not just durations.

**Q3. Design the cut.**
Take your six-shot sequence from Drill 2 Q2. Assign each shot a duration in seconds to total 25s. Mark where each cut lands relative to a 120bpm music track (beats are every 0.5s). Identify one cut that should be a cut-on-action, one that should be a J cut, and one where you'd cut the music to silence — and justify each.

**Q4. The muted test.**
Describe your video's first two seconds. Then answer: if a viewer sees only those two seconds, with no sound, at arm's length on a phone, what do they now know? If the answer is "that something is happening," redesign it and explain what you changed. Then list three specific ways a logo-first opening loses viewers.

**Q5. Deliverable matrix.**
Build the full deliverable matrix for a product launch across three platforms of your choosing. For each cell state ratio, duration, caption treatment, and what gets cut relative to the master. Then identify which single shot in your sequence is most likely to break when reframed to 9:16, and describe both the fix and the thing you'd have done at generation time to avoid it.

**Q6. Reading / doing.**
Take any three clips you have (or generate three) and cut them together three different ways: (i) in a random order with equal 3-second durations, (ii) in your best order with varied durations, cutting on action, (iii) the same as (ii) but with a music bed and cuts snapped to the beat. Watch all three back to back.
- Which change produced the largest jump in perceived quality, and why, mechanically?
- Where did cutting on action succeed or fail, and what property of the clips determined that?
- What did adding music force you to change about the durations you'd chosen?

---

# Lesson 4: The Pipeline — Building With AI Video in Code

## 4.1 Why This Lesson Exists

Here is how the first AI video project goes for nearly everyone. You open the tool. You type a prompt. You get a clip. It's 80% right, so you re-roll. Now it's 80% right in a different way. You re-roll again. Forty minutes and a meaningful pile of credits later you have eleven clips, no naming convention, no record of which prompt produced which file, no idea which of the eleven you liked at minute six, and — because generation is non-deterministic — no way to get that one back.

This is the **vibes** trajectory, and it is expensive in a way that prompt-only work with text is not. Text generation costs fractions of a cent and returns in a second. Video generation costs real money per clip and returns in minutes. **The economics force discipline that text work lets you skip.**

The discipline is to treat video generation as a build pipeline: an input spec you can version, deterministic-where-possible steps, artifacts on disk with stable names, a manifest that records what produced what, and an acceptance check at each stage before you spend money on the next one. This is exactly the shape of a CI pipeline, which is why doing it from a coding agent is a natural fit rather than a novelty.

## 4.2 The Cost Gradient: Where to Fail Cheaply

Order every step by what it costs to redo, and push every decision as far up the cheap end as possible:

```
CHEAP  ----------------------------------------------------->  EXPENSIVE
       text        still         short          long         full
       (script,    image         video clip     video clip   re-shoot
       shot list)  generation    (i2v, 4s)      (t2v, 10s)   of everything

       $0.00       $0.0x         $0.x           $x           $$$
       seconds     seconds       minutes        minutes      hours
```

The single most important consequence: **every decision you can make in text, make in text.** Argue about the shot list in a markdown file. Argue about composition on generated stills. Only when a still is approved does it become a video job. A team that reviews a written shot list and an approved still frame for every shot before generating any video will spend a fraction of what a team that "just tries stuff" spends, and get better results.

Second consequence: **approve in stages, with an explicit gate at each one.** Script → shot list → stills → clips → assembly → mix → export. Never let an unapproved artifact flow into a more expensive stage.

## 4.3 Generation Modes and Control Surfaces

Know what levers exist, because they're the difference between steering and wishing:

- **Text-to-video** — description in, clip out. Maximum variance, minimum control. Use for abstract or background material where "some plausible version of this" is fine.
- **Image-to-video** — a still plus a motion description. The workhorse (2.8). Look is locked; you're only rolling dice on motion.
- **First-and-last-frame** — supply both endpoints; the model interpolates between them. The strongest control available. Use it whenever a shot must end in a specific state — especially to hand off cleanly into the next shot.
- **Reference / identity conditioning** — a trained character reference or a set of reference images that pin a face or object's appearance across many generations. The only real answer to "same person in every shot."
- **Video-to-video / restyle** — an existing clip in, restyled clip out. Motion is preserved; look changes.
- **Seeds** — where exposed, the seed makes a generation reproducible. Record it. A clip you can't regenerate is a clip you can't iterate on.
- **Duration, aspect ratio, resolution** — parameters, never prose (2.6).

And the technique that ties shots together, which costs nothing:

```bash
# Extract the final frame of clip 01 to use as the start image of clip 02
ffmpeg -sseof -0.05 -i shot01.mp4 -frames:v 1 -q:v 1 shot01_last.png
```

Feed `shot01_last.png` as the start image for shot 02, and the two clips join seamlessly — you can cut between them invisibly, or concatenate them into one longer continuous move than either model call could produce alone.

## 4.4 ffmpeg: Your Assembly Layer

Everything an editor does to files, `ffmpeg` does from a command line — which means an agent can do it. The commands worth knowing by heart:

```bash
# Inspect anything (always your first move on an unknown file)
ffprobe -hide_banner file.mp4

# Normalize every generated clip to one spec before assembling
# (this is the step that prevents 90% of downstream weirdness)
ffmpeg -i in.mp4 -r 30 -s 1080x1920 -c:v libx264 -crf 18 -preset slow \
       -pix_fmt yuv420p -c:a aac -ar 48000 norm.mp4

# Trim by in/out point, re-encoding for frame accuracy
ffmpeg -ss 00:00:01.20 -to 00:00:04.00 -i in.mp4 -c:v libx264 -crf 18 out.mp4

# Concatenate clips that share identical codec/resolution/fps
printf "file 'shot01.mp4'\nfile 'shot02.mp4'\n" > list.txt
ffmpeg -f concat -safe 0 -i list.txt -c copy assembled.mp4

# Reframe 16:9 -> 9:16 by filling then cropping (check the result by eye)
ffmpeg -i wide.mp4 -vf "scale=1080:1920:force_original_aspect_ratio=increase,\
crop=1080:1920" -c:a copy vertical.mp4

# Mix a voice track and a music bed, ducking the music under the voice
ffmpeg -i picture.mp4 -i voice.wav -i music.wav -filter_complex \
  "[2:a][1:a]sidechaincompress=threshold=0.05:ratio=8[duck];\
   [1:a][duck]amix=inputs=2:duration=first[a]" \
  -map 0:v -map "[a]" -c:v copy -c:a aac mixed.mp4

# Normalize loudness for delivery (check your platform's current target)
ffmpeg -i mixed.mp4 -af loudnorm=I=-14:TP=-1.5:LRA=11 -c:v copy final.mp4

# Burn in captions
ffmpeg -i final.mp4 -vf "subtitles=captions.srt:force_style='FontSize=18'" \
       -c:a copy captioned.mp4

# Pull a thumbnail
ffmpeg -ss 00:00:01.5 -i final.mp4 -frames:v 1 -q:v 2 thumb.jpg
```

Two habits that matter:

**Normalize before you assemble.** Clips from different models will differ in frame rate, resolution, pixel format, and color range. Running every clip through one identical normalization command first turns a class of baffling assembly bugs (audio drift, judder, sudden color shifts at a cut) into a non-problem.

**Encode to delivery once, at the end** (1.5). Keep intermediates at high quality; the `-crf 18` in the commands above is deliberately conservative for that reason.

## 4.5 Project Structure and the Manifest

A layout that survives contact with a real project:

```
launch-video/
  brief.md              # audience, goal, tone, CTA, constraints. Written first.
  shots.md              # the shot list. Reviewed BEFORE anything is generated.
  manifest.json         # machine-readable record of every shot + its provenance
  assets/               # things you own: logo, product photos, fonts, music
  stills/               # generated frames, approved and rejected
  clips/raw/            # exactly what the model returned. Never edited in place.
  clips/norm/           # normalized to project spec
  audio/                # vo.wav, music.wav, sfx/
  captions/             # transcript.txt, captions.srt
  out/                  # master.mp4 + derived deliverables
  CLAUDE.md             # standing rules for the agent
```

The manifest is the thing that makes the project reproducible:

```json
{
  "project": "launch-v1",
  "spec": { "fps": 30, "resolution": "1080x1920", "aspect": "9:16" },
  "lighting_clause": "soft key from window left, cool ambient fill, dark neutral background",
  "shots": [
    {
      "id": "s01",
      "beat": "hook",
      "size": "ECU",
      "prompt": "Extreme close-up of ... camera slowly pushes in ...",
      "mode": "i2v",
      "start_image": "stills/s01_v3.png",
      "model": "<model-id>",
      "seed": 41822,
      "duration": 3.0,
      "raw": "clips/raw/s01.mp4",
      "in": 0.4, "out": 2.8,
      "status": "approved",
      "notes": "v1 and v2 drifted on the logo; v3 locked it"
    }
  ]
}
```

Why this pays for itself immediately: it makes "regenerate shot 4 with a slower push" a one-line change, it records *which* still produced which clip, it keeps the rejected takes explicable, and it lets an agent rebuild the whole video from source with one command. It is also, not incidentally, the exact structure a coding agent is good at maintaining and terrible at inventing on the fly.

## 4.6 Acceptance Criteria: Evaluating Non-Deterministic Output

You cannot assert `output == expected` on a generated clip, for the same reason you can't with generated text: the output differs every run and there's no single correct result. So you evaluate *qualities*, per shot, against criteria written **before** you look at the output.

A workable per-shot checklist:

- **Subject fidelity** — is the product/person actually what it should be? Logo correct? Proportions right?
- **Motion integrity** — does anything morph, warp, or gain/lose limbs or edges? Watch frame by frame at the point of fastest motion; that's where models break.
- **Continuity** — light direction, color temperature, and screen direction consistent with adjacent shots?
- **Physics** — do objects have weight? Does fabric, liquid, and hair behave? This is where "AI slop" is usually detected by viewers who can't name why.
- **Usability** — is there a clean 2+ second window inside the clip you can actually cut with? A clip that's only good for 0.4s is a failed clip even if that 0.4s is gorgeous.
- **Frame ends** — are the first and last frames clean enough to cut on or chain from?

Write these criteria in `shots.md` per shot. Then the review is a checklist, not a vibe, and "we need another take" becomes a specific instruction ("logo distorts from 1.8s; re-roll with the product locked as a reference image") instead of "try again."

**Track cost per shot.** Log credits or dollars in the manifest alongside the take number. Three weeks in, that record is how you learn that a certain model is twice the price for a marginal quality gain on your kind of content, which is a decision you cannot make from memory.

## 4.7 Working With a Coding Agent

The pipeline above is why this work fits a coding agent well: it's file manipulation, API polling, JSON bookkeeping, and shell commands, wrapped around a handful of creative decisions that stay yours.

What to delegate:
- Maintaining the manifest and directory structure.
- Expanding an approved shot list into full formula-compliant prompts (2.6), with the lighting clause pasted identically into each.
- Firing generation jobs, polling async results, downloading, and naming outputs.
- Normalization, trimming, concatenation, reframing, mixing, captioning, export — all the `ffmpeg` work.
- Producing the deliverable matrix from one master.

What to keep:
- The brief and the shot list. If you delegate what the video is *for*, you will get a generic video, because generic is the average of the training data.
- Every approval gate (4.2). The agent should stop and show you stills before generating video, and clips before assembly.
- Final judgment on rhythm and cut points. You can automate a first assembly; you cannot automate whether it lands.

Two rules to put in your `CLAUDE.md` on day one:

```
- Never generate a video job without an approved still and an entry in
  manifest.json. Show me the shot list and the stills first.
- Record model, seed, prompt, and cost for every generation. Never overwrite
  a file in clips/raw/ — new takes get a new version suffix.
```

The first prevents burning credits on unreviewed work. The second prevents the situation where the good take is gone and nobody knows what made it.

## 4.8 Summary: The Rules

1. **Video generation is a build pipeline**, not a chat. Spec in, artifacts on disk, manifest recording provenance.
2. **Fail at the cheap end.** Decide in text, iterate on stills, generate video only on approved input.
3. **Gate every stage.** Script → shots → stills → clips → assembly → mix → export, with an explicit approval between each.
4. **Use the strongest control surface available**: i2v over t2v, first-last-frame over i2v, reference conditioning for identity. Record seeds.
5. **Normalize every clip to one spec before assembly**, and encode to delivery exactly once.
6. **The manifest is the project.** Prompt, model, seed, in/out, status, cost, per shot.
7. **Write acceptance criteria before you look at the output.** Review by checklist; re-roll with a specific diagnosis.
8. **Delegate the bookkeeping and the ffmpeg; keep the brief, the gates, and the rhythm.**

## 4.9 Drill 4

Rules: show the pipeline, the artifacts, and the gates — not "I'd be organized about it." Write your answers down.

**Q1. Cost the pipeline.**
You need a 30s launch video, 6 shots, in 9:16 and 16:9. Estimate the number of generation calls for two approaches: (a) text-to-video, re-rolling until each shot is right, and (b) stills-first image-to-video with approval gates. State your assumptions about re-roll rates explicitly. Then identify the one place approach (b) is *more* expensive, and argue whether it's worth it.

**Q2. Design the manifest.**
Write the full JSON schema you'd use for a project with 8 shots, two aspect ratios, and a voiceover. It must support: multiple takes per shot, a record of why each take was rejected, cost tracking, and enough information to rebuild every deliverable from source with one command. Then describe the rebuild command's steps in order.

**Q3. Acceptance criteria.**
Write per-shot acceptance criteria for these three shots, before generation: (a) an ECU of your product's surface, (b) a medium shot of a person using it, (c) a wide establishing shot. Each must have at least four checkable criteria. Then explain why "it looks good" fails as a criterion in a way that "no morphing on the logo between 0.0s and 3.0s" does not.

**Q4. Debug the assembly.**
Your assembled video has: audio drifting out of sync by the end, one shot that judders, and a visible color shift at the third cut. For each symptom, name the most likely cause from Lessons 1 and 4, the command or check that confirms it, and the fix. Then name the single upstream step that would have prevented all three.

**Q5. Agent boundaries.**
Write the `CLAUDE.md` for this project: the standing rules, the approval gates, the naming conventions, the things the agent must never do, and the format it should use when presenting work for review. Then justify each rule by naming the specific failure it prevents. At least two rules must be about cost.

**Q6. Reading / doing.**
Build the smallest possible end-to-end version: write a three-line brief, a three-shot list, generate three stills, animate them, normalize all three with the same ffmpeg command, concatenate them, add any music track, and export one file. Do it with the manifest, even though three shots doesn't "need" one.
- Which stage took the longest, and was it the stage you expected?
- Where did your shot list turn out to be underspecified once you saw the stills?
- What did the normalization step change about the raw clips? Run `ffprobe` before and after and compare.

---

## Phase 1 Master Rules

### What video is

- Stills plus a clock. Pictures-per-second and data-per-picture determine everything else.
- Frame rate carries cultural meaning; pick one per project and never mix.
- Aspect ratio is a composition decision made *before* generation, not a crop applied after.
- Codec ≠ container ≠ bitrate. Lossy re-encoding is a one-way door; encode to delivery once.
- Color is a pipeline: correct, then grade, once, over the whole timeline.
- Audio is half the video and most of the perceived quality.
- The model gives you clips. Cuts, continuity, text, sync, sound, and duration are yours.

### Shots

- Specify, don't judge. Film vocabulary works; marketing adjectives don't.
- Shot size is emotional distance; vary it deliberately across the video.
- Lens sets perspective; shallow depth of field is the cheapest premium signal.
- Lighting = direction + quality + ratio. Name all three, identically, in every shot of a sequence.
- One dominant camera move per clip. Two moves is two shots and a cut.
- Continuity is engineered: reference images for identity, consistent screen direction, one lighting clause.
- Image-to-video is the real technique. Iterate on stills; ask the video model only for motion.

### Editing

- Meaning is made at the cut. Same clips, different order, different video.
- Hard cut is the default; cut on action; J and L cuts to smooth; transitions announce themselves.
- Shot length is pacing. Vary it, and snap cuts to the music's beat grid.
- The first two seconds must work muted and start on your most arresting frame.
- Burn in captions; composite real text in post.
- Voice > music > SFX. Duck the music. Use silence deliberately.
- Cut longest first, derive the rest, keep a master, verify every reframe by eye.

### Pipeline

- Treat generation as a build: spec in, artifacts on disk, manifest recording provenance.
- Fail at the cheap end; gate every stage; never let unapproved work flow into an expensive step.
- Use the strongest control surface available and record seeds.
- Normalize every clip to one spec before assembly.
- Write acceptance criteria before you look at the output; re-roll with a diagnosis.
- Delegate bookkeeping and ffmpeg; keep the brief, the gates, and the rhythm.

### Success criteria

After Phase 1 you should be able to:

- Explain to a skeptical colleague, mechanically, why a video "feels cheap" — naming frame rate, lighting, sound, or pacing rather than gesturing at quality.
- Write a shot prompt in which every property that determines the image has been specified, and say what each clause is doing.
- Take six unrelated clips and cut them into something with a hook, a rhythm, and a point.
- Run a generation project as a pipeline with artifacts, gates, and a manifest, and know what each stage cost.

If you can do those four things, you can direct AI video instead of gambling with it.

---

# The Whole Picture: How One Launch Video Actually Gets Made

Zoom out. Here is everything from this phase as a single video's journey — from an intent to a set of deliverables. **Almost every box is a decision you make. The model is one box in the middle.**

```
        WHAT IS THIS VIDEO FOR?
        (audience, one goal, one CTA, tone)   -- brief.md
            |
            v
[ SHOT LIST  -  Lesson 2 ]
    beats -> shots. size, lens, move, action, ONE
    shared lighting clause. Acceptance criteria per shot.
    ** cheapest place to be wrong: be wrong here **
            |
            v
[ STILLS  -  Lesson 2.8 ]
    generate frames, iterate freely, approve the LOOK
    before any video money is spent
            |   approved stills + motion prompts
            v
[ THE GENERATOR  -  Lesson 1.8 ]
    non-deterministic . short clips . no memory
    gives you material, not a video
            |   raw clips
            v
[ NORMALIZE  -  Lessons 1.3-1.5, 4.4 ]
    one fps, one resolution, one pixel format
    (skip this and everything downstream is haunted)
            |
            v
[ EDIT  -  Lesson 3 ]
    order, trim, cut on action, J/L cuts, pacing,
    beat-matched to the music. THIS is where the video is made.
            |
            v
[ SOUND + TEXT  -  Lessons 3.5-3.6 ]
    voice > music > SFX, ducked. Real typefaces.
    Burned-in captions. Safe areas respected.
            |
            v
[ GRADE + MASTER  -  Lesson 1.6, 3.7 ]
    one look over the whole timeline; export ONE
    high-quality master
            |
            v
[ DERIVE DELIVERABLES  -  Lesson 3.7 ]
    ratios x durations, each reframe checked by eye,
    each encoded from the master exactly once
            |
            v
        PUBLISHED
```

*Almost every box is a decision you make. The model is one box in the middle — non-deterministic, short-form, memoryless.*

## Quick Reference Table

| Concept | What it is | Who controls it | Main quality lever |
|---|---|---|---|
| **Frame rate** | Images per second | You, once, per project | Pick one; never mix; 24 = film, 30 = web |
| **Aspect ratio** | Frame shape | You, *before* generating | Compose for the primary ratio; respect safe areas |
| **Codec / bitrate** | Compression and data rate | You, at export | Encode to delivery exactly once |
| **Shot size** | How much of the subject is framed | Your prompt | Vary it; it's emotional distance |
| **Lens / DoF** | Field of view and focus depth | Your prompt | Shallow DoF = cheapest premium signal |
| **Lighting** | Direction + quality + ratio | Your prompt | Identical clause across a sequence |
| **Camera move** | How the frame travels | Your prompt | Exactly one per clip |
| **The cut** | The join between clips | You, in the edit | Cut on action; snap to the beat |
| **Pacing** | Shot durations over time | You, in the edit | Vary it; open fast, land slow |
| **Sound mix** | Voice, music, SFX | You | Duck music under voice; use silence |
| **The generator** | Short-clip producer | The model (you steer) | i2v > t2v; reference images; record seeds |
| **The manifest** | Provenance of every artifact | You | Makes the project rebuildable |

## The Mental Model in One Sentence

> **An AI video model is a non-deterministic, memoryless producer of short, look-inconsistent clips; making a video is the discipline of deciding what the clips must be before they exist, controlling them with images rather than adjectives, and then building the actual video — the order, the rhythm, the sound, the text, the exports — out of material the model was never able to give you.**

---

*Phase 1 complete. Phase 2: multi-scene narrative, character and product consistency at scale, motion and VFX compositing, automated assembly, and production operations — cost, rights, review workflows, and versioning.*
