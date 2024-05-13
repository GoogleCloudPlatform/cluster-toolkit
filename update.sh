#!/bin/bash

# Check for correct number of arguments
if [ $# -ne 2 ]; then
    echo "Usage: update.sh <old_filename> <new_filename>"
    exit 1
fi

# Get parameters
old_filename="$1"
new_filename="$2"

# Find and rename files
# find . -type f -name "$old_filename" -exec sh -c 'mv "$1" "${1%$old_filename}$new_filename"' _ {} \;
find . -type f -name "$old_filename" -exec sh -c 'echo "file found $1 and will be renamed as $2";mv "$1" "${1%/*}/$2"' _ {} "$new_filename" \;


# Replace content in files
# find . -type f -name "$new_filename" -exec sed -i "s/$old_filename/$new_filename/g" {} \; 
git sed $old_filename $new_filename
