#!/bin/bash

apt update

apt install -y --no-install-recommends golang-go

renovate
