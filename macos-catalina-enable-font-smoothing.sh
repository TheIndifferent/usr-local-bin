#!/bin/bash

find /Applications -name 'Info.plist' \
    -exec grep --binary-files=without-match '\.helper' {} ';' \
    | sed -e 's;^.*<string>;;g' -e 's;</string>;;g' \
    | sort \
    | uniq \
    | xargs -I % defaults write % CGFontRenderingFontSmoothingDisabled -bool NO
