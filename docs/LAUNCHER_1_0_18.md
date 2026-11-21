# Launcher 1.0.18: transferable deployment configuration

The native core accepts an operator-supplied `PurpleMagic.profile.json` next to the original portable EXE. Portable launch location is passed through the existing DNF90_LAUNCH_DIRECTORY environment variable. Direct core runs look beside the core. If no sidecar exists, the approved embedded profile remains the fallback.

The sidecar is a local deployment trust root containing the public signing key and pinned certificate, not an online update. Distribute the EXE and profile together from the operator. Never accept replacement trust from an unverified network response. No private signing key or database credential belongs in this file. Link/reparse paths and profiles larger than 64 KiB are rejected.

Default state directories are partitioned by API origin and public signing key, so a rebuilt server or another operator does not consume an old server's cached bootstrap. Explicit --state-directory behavior is preserved. The read-only --deployment-info switch prints API origin, EXE/PVF hashes and the public-key digest for operator validation; it does not connect or start the UI.

The deployment tool packages EXE and configuration as ServerS5A3-1.0.18.zip, using the gateway's existing allowed release filename. Existing single-file deployments keep their embedded-profile fallback. Game/PVF bytes are unchanged.
