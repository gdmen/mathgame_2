# Ubuntu 22.04
sudo apt-get update
sudo apt install golang-go
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

# Refresh let's encrypt cert
sudo certbot renew
sudo service mathgame-web restart

# Production logs
journalctl -u mathgame-api -b
journalctl -u mathgame-web -b
