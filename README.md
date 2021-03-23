# skadi-agent-shell
A demo of skadi agent binary running in linux shell.

## Install
Before we release the deb/rpm/homebrew packages,
it can only install the agent manually.

Download the release and unzip it.  
Edit the config file `skadi.yml`, put your agent `TOKEN` in it.
Now you can use `./skadi` run it for testing.

Then, if everything is ok, you want to deploy it,
```shell
mv skadi /usr/bin
mkdir /etc/skadi
mv skadi.yml /etc/skadi/
```
If the user is not root, use sudo.

## Systemd
First, move the service to system path:
```shell
mv skadi.service /etc/systemd/system/
```
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
