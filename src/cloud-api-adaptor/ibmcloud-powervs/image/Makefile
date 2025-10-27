#
# SPDX-License-Identifier: Apache-2.0
#

SKOPEO_SRC =
SKOPEO_BIN = /usr/bin/skopeo
SETUP_SFTP ?= no

export USER_NAME ?= peerpod
export DISABLE_SSH_LOGIN ?= false
export FORWARDER_PORT ?=

ROOT_DIR := $(dir $(lastword $(MAKEFILE_LIST)))../../
include $(ROOT_DIR)podvm/Makefile.inc

.PHONY: build clean

prerequisites: ## Install dependencies
	./prereq.sh

build: prerequisites $(BINARIES)
	bash -x ./copy-files.sh
ifeq ($(SETUP_SFTP),yes)
	bash -x ./setup-sftp.sh
endif

clean:
	rm -f $(BINARIES)
	rm -rf "$(UMOCI_SRC)" "$(PAUSE_SRC)" "$(FILES_DIR)/$(PAUSE_BUNDLE)"
	yum remove -y skopeo

.PHONY: force
force:
