# wintoolbar

> A taskbar widget for Windows 11

![wintoolbar in the Windows 11 taskbar](assets/taskbar.png)

## Features

- Audio visualizer
- Media controls
- Network monitor

## Platform

Windows 11. Windows 10 not tested.

## Install

Download from [Releases](https://github.com/achmadss/wintoolbar/releases/latest).

## Config file

Settings write `%APPDATA%\wintoolbar\config.json`. You can edit it by hand and restart, or edit it while running and reopen Settings. A missing file is written with defaults on first run.

```json
{
  "visible": true,
  "layout": {
    "left": [],
    "center": ["visualizer", "media"],
    "right": ["network"],
    "leftOffset": 12,
    "centerOffset": 0,
    "rightOffset": 12
  },
  "visualizer": {
    "enabled": true,
    "bars": 10,
    "centered": false,
    "baseline": false,
    "baselineAutoHide": false,
    "sensitivity": 2,
    "peak": 3,
    "color": "",
    "albumColor": true
  },
  "media": { "enabled": true, "controls": true, "stop": false },
  "network": { "enabled": true, "adapter": "", "bits": false }
}
```

## License

MIT. See [LICENSE](LICENSE).
