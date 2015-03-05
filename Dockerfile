FROM dockerfile/nodejs:latest

ADD . /data/

CMD node index.js
