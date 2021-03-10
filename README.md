# skadi-agent-shell
A demo of skadi agent binary running in linux shell.

## Systemd
After saving your service file, you can start the service for the first time with the usual systemctl dance:
```shell
sudo systemctl daemon-reload
sudo systemctl enable skadi
sudo systemctl start skadi
```

Verify that it is running:
```shell
systemctl status skadi
```

When running with our official service file, its output will be redirected to journalctl:
```shell
journalctl -u skadi --no-pager | less
```
