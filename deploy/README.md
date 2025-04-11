# Ubuntu 22.04
sudo apt-get purge golang*
sudo rm -rf /usr/local/go
wget -c https://go.dev/dl/go1.24.2.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.24.2.linux-amd64.tar.gz
```
# set PATH so it includes /usr/local/go/bin if it exists
if [ -d "/usr/local/go/bin" ] ; then
    PATH="/usr/local/go/bin:$PATH"
fi
```
sudo apt install make

sudo apt-get install nodejs npm
sudo npm install -g serve

# Using let's encrypt
sudo apt-get remove certbot
sudo ln -s /snap/bin/certbot /usr/bin/certbot
sudo certbot certonly --standalone

git clone https://github.com/gdmen/mathgame_2.git
cd mathgame_2
make
sudo cp deploy/mathgame-api.service /etc/systemd/system
sudo cp deploy/mathgame-web.service /etc/systemd/system

sudo systemctl daemon-reload
sudo systemctl enable mathgame-api
sudo systemctl enable mathgame-web
sudo service mathgame-api start
sudo service mathgame-web start

# Updates
cd mathgame_2
git fetch --all
git reset --hard origin/master
make
sudo cp deploy/mathgame-api.service /etc/systemd/system
sudo cp deploy/mathgame-web.service /etc/systemd/system
sudo systemctl daemon-reload
sudo service mathgame-api restart
sudo service mathgame-web restart
! MANUALLY RUN NEW MIGRATIONS FROM server/api/migrations !

# Refresh let's encrypt cert
sudo certbot renew
sudo service mathgame-web restart

# Production logs
journalctl -u mathgame-api -b
journalctl -u mathgame-web -b
