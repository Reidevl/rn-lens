# Lens example app

A minimal Expo app used to try Lens by hand and to record protocol fixtures.
It logs a line every two seconds, fetches JSON on demand and can throw an
uncaught error on demand.

It is intentionally **not** part of the pnpm workspace: it uses npm like most
React Native projects do.

## Run

```bash
npm install
npx expo start          # press i for the iOS simulator, a for Android
```

Then open Lens (`task dev` from the repository root). The app appears in the
Targets panel within a couple of seconds.

## Record fixtures

```bash
rnlens targets                                   # find the target id
rnlens record -target "<id>" -out ../../internal/adapters/cdpws/testdata/rn088-ios.jsonl -duration 20s
```
