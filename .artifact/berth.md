---
title: Berth
date: 2026-10
status: released
category: tool
tags:
  - macos
url: https://berth.computer
links:
  source: https://github.com/arfct/berth
media:
  - src: site.png
    caption: The Berth site, with a Mac mini above the output of a deploy
    alt: A cream web page with the headline Your agent built it. Berth gives it a home, a drawing of a Mac mini with a Berth sticker on its front, and a dark terminal window listing the steps of a deploy
credits:
  - dps
unconfirmed: [date, status]
---

Berth was written so that the small web apps its author and coding agents kept making could go live on a push, as on a hosting service, without a monthly bill for each. Its host is one Apple Silicon Mac, ideally a Mac mini. A push to an app's GitHub repository builds the app, starts the release in its own Linux virtual machine, checks that it answers, and then moves traffic to it. Each app gets a public address on the owner's domain through a Cloudflare tunnel and a second address on the local network, and no ports are opened on the router.

Every app also has a dev version with its own data, built so that coding agents can test their own work. One command deploys the working tree as it stands on disk, uncommitted edits included, in about five seconds for a small change, and its exit code tells an agent whether the build failed, the app did not start or a migration failed. Postgres databases are dumped every night, and Google sign-in can be put in front of any app. An admin page charts traffic from the request logs, and the Mac's load. A shell command can be set to run on each page view, which the README uses to blink a USB light.

Everything runs on one Mac, with no failover. The site at berth.computer is served by Berth from a Mac mini, and Homebrew installs version 0.2.0, which requires macOS 26.

Go, with Apple's container runtime, Caddy and cloudflared. 2026.
