#!/bin/sh
set -e

if [ "$1" = "configure" ] || [ "$1" = "abort-upgrade" ] || [ "$1" = "abort-deconfigure" ] || [ "$1" = "abort-remove" ] ; then
	# This will only remove masks created by d-s-h on package removal.
	deb-systemd-helper unmask skadi.service >/dev/null || true

	# was-enabled defaults to true, so new installations run enable.
	if deb-systemd-helper --quiet was-enabled skadi.service; then
		# Enables the unit on first installation, creates new
		# symlinks on upgrades if the unit file has changed.
		deb-systemd-helper enable skadi.service >/dev/null || true
		# don't start after first install, need token config
		# deb-systemd-invoke start skadi.service >/dev/null || true
	else
		# Update the statefile to add new symlinks (if any), which need to be
		# cleaned up on purge. Also remove old symlinks.
		deb-systemd-helper update-state skadi.service >/dev/null || true
	fi

	# Restart only if it was already started
	if [ -d /run/systemd/system ]; then
		systemctl --system daemon-reload >/dev/null || true
		if [ -n "$2" ]; then
			deb-systemd-invoke try-restart skadi.service >/dev/null || true
		fi
	fi
fi