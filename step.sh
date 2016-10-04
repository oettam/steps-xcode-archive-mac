#!/bin/bash

THIS_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

set -e

#=======================================
# Functions
#=======================================

RESTORE='\033[0m'
RED='\033[00;31m'
YELLOW='\033[00;33m'
BLUE='\033[00;34m'
GREEN='\033[00;32m'

function color_echo {
	color=$1
	msg=$2
	echo -e "${color}${msg}${RESTORE}"
}

function echo_fail {
	msg=$1
	echo
	color_echo "${RED}" "${msg}"
	exit 1
}

function echo_warn {
	msg=$1
	color_echo "${YELLOW}" "${msg}"
}

function echo_info {
	msg=$1
	echo
	color_echo "${BLUE}" "${msg}"
}

function echo_details {
	msg=$1
	echo "  ${msg}"
}

function echo_done {
	msg=$1
	color_echo "${GREEN}" "  ${msg}"
}

function validate_required_input {
	key=$1
	value=$2
	if [ -z "${value}" ] ; then
		echo_fail "[!] Missing required input: ${key}"
	fi
}

function validate_required_input_with_options {
	key=$1
	value=$2
	options=$3

	validate_required_input "${key}" "${value}"

	found="0"
	for option in "${options[@]}" ; do
		if [ "${option}" == "${value}" ] ; then
			found="1"
		fi
	done

	if [ "${found}" == "0" ] ; then
		echo_fail "Invalid input: (${key}) value: (${value}), valid options: ($( IFS=$", "; echo "${options[*]}" ))"
	fi
}

#=======================================
# Main
#=======================================

#
# Validate parameters
echo_info "Configs:"
echo_details "* workdir: ${workdir}"
echo_details "* project_path: ${project_path}"
echo_details "* scheme: ${scheme}"
echo_details "* configuration: ${configuration}"
echo_details "* output_dir: ${output_dir}"
echo_details "* force_code_sign_identity: ${force_code_sign_identity}"
echo_details "* force_provisioning_profile: ${force_provisioning_profile}"
echo_details "* export_options_path: ${export_options_path}"
echo_details "* export_method: ${export_method}"
echo_details "* is_clean_build: ${is_clean_build}"
echo_details "* output_tool: ${output_tool}"
echo_details "* is_export_xcarchive_zip: ${is_export_xcarchive_zip}"
echo_details "* is_export_all_dsyms: $is_export_all_dsyms"

echo

validate_required_input "project_path" $project_path
validate_required_input "scheme" $scheme
validate_required_input "is_clean_build" $is_clean_build
validate_required_input "output_dir" $output_dir
validate_required_input "output_tool" $output_tool
validate_required_input "is_export_xcarchive_zip" $is_export_xcarchive_zip

options=("xcpretty"  "xcodebuild")
validate_required_input_with_options "output_tool" $output_tool "${options[@]}"

options=("yes"  "no")
validate_required_input_with_options "is_clean_build" $is_clean_build "${options[@]}"
validate_required_input_with_options "is_export_xcarchive_zip" $is_export_xcarchive_zip "${options[@]}"

if [ ${is_force_code_sign} != "no" ] ; then
	echo_warn "is_force_code_sign is deprecated!"
	echo_warn "Use `force_code_sign_identity` and `force_provisioning_profile` instead."
fi

# Detect Xcode major version
xcode_major_version=""
major_version_regex="Xcode ([0-9]).[0-9]"
out=$(xcodebuild -version)
if [[ "${out}" =~ ${major_version_regex} ]] ; then
	xcode_major_version="${BASH_REMATCH[1]}"
fi

if [ "${xcode_major_version}" -lt "6" ] ; then
	echo_fail "Invalid xcode major version: ${xcode_major_version}, should be greater then 6"
fi

IFS=$'\n'
xcodebuild_version_split=($out)
unset IFS

xcodebuild_version="${xcodebuild_version_split[0]} (${xcodebuild_version_split[1]})"
echo_details "* xcodebuild_version: $xcodebuild_version"

# Detect xcpretty version
xcpretty_version=""
if [[ "${output_tool}" == "xcpretty" ]] ; then
	xcpretty_version=$(xcpretty --version)
	exit_code=$?
	if [[ $exit_code != 0 || -z "$xcpretty_version" ]] ; then
		echo_fail "xcpretty is not installed
		For xcpretty installation see: 'https://github.com/supermarin/xcpretty',
		or use 'xcodebuild' as 'output_tool'.
		"
	fi

	echo_details "* xcpretty_version: $xcpretty_version"
fi

# export_options_path & Xcode 6
if [ ! -z "${export_options_path}" ] && [[ "${xcode_major_version}" == "6" ]] ; then
	echo_warn "xcode_major_version = 6, export_options_path only used if xcode_major_version > 6"
	export_options_path=""
fi

# Project-or-Workspace flag
if [[ "${project_path}" == *".xcodeproj" ]]; then
	CONFIG_xcode_project_action="-project"
elif [[ "${project_path}" == *".xcworkspace" ]]; then
	CONFIG_xcode_project_action="-workspace"
else
	echo_fail "Failed to get valid project file (invalid project file): ${project_path}"
fi
echo_details "* CONFIG_xcode_project_action: $CONFIG_xcode_project_action"

export_format="app"
if [[ "${export_method}" == "app-store" ]]; then
	export_format="pkg"
fi
echo_details "* export_format: $export_format"

echo

# abs out dir pth
mkdir -p "${output_dir}"
cd "${output_dir}"
output_dir="$(pwd)"
cd -

# output files
archive_tmp_dir=$(mktemp -d -t bitrise-xcarchive)
archive_path="${archive_tmp_dir}/${scheme}.xcarchive"
echo_details "* archive_path: $archive_path"

file_path="${output_dir}/${scheme}.${export_format}"
echo_details "* file_path: $file_path"

dsym_zip_path="${output_dir}/${scheme}.dSYM.zip"
echo_details "* dsym_zip_path: $dsym_zip_path"

# work dir
if [ ! -z "${workdir}" ] ; then
	echo_info "Switching to working directory: ${workdir}"
	cd "${workdir}"
fi

#
# Main

#
# Bit of cleanup
if [ -e "${file_path}" ] ; then
	echo_warn "App at path (${file_path}) already exists - removing it"
	if [ $export_format == "app" ] ; then
		rm -rf "${file_path}"
	else
		rm "${file_path}"
	fi
fi

#
# Create the Archive with Xcode Command Line tools
echo_info "Create the Archive ..."

archive_cmd="xcodebuild ${CONFIG_xcode_project_action} \"${project_path}\""
archive_cmd="$archive_cmd -scheme \"${scheme}\""

if [ ! -z "${configuration}" ] ; then
	archive_cmd="$archive_cmd -configuration \"${configuration}\""
fi

if [[ "${is_clean_build}" == "yes" ]] ; then
	archive_cmd="$archive_cmd clean"
fi

archive_cmd="$archive_cmd archive -archivePath \"${archive_path}\""

if [[ -n "${force_provisioning_profile}" ]] ; then
	echo_details "Forcing Provisioning Profile: ${force_provisioning_profile}"

	archive_cmd="$archive_cmd PROVISIONING_PROFILE=\"${force_provisioning_profile}\""
fi

if [[ -n "${force_code_sign_identity}" ]] ; then
	echo_details "Forcing Code Signing Identity: ${force_code_sign_identity}"

	archive_cmd="$archive_cmd CODE_SIGN_IDENTITY=\"${force_code_sign_identity}\""
fi

if [[ "${output_tool}" == "xcpretty" ]] ; then
	archive_cmd="set -o pipefail && $archive_cmd | xcpretty"
fi

echo_details "$ $archive_cmd"
echo

eval $archive_cmd

# ensure xcarchive exists
if [ ! -e "${archive_path}" ] ; then
    echo_fail "no archive generated at: ${archive_path}"
fi

#
# Exporting the ipa with Xcode Command Line tools
echo_info "Exporting APP from generated Archive ..."

# You'll get a "Error Domain=IDEDistributionErrorDomain Code=14 "No applicable devices found."" error
# if $GEM_HOME is set and the project's directory includes a Gemfile - to fix this
# we'll unset GEM_HOME as that's not required for xcodebuild anyway.
# This probably fixes the RVM issue too, but that still should be tested.
# See also:
# - http://stackoverflow.com/questions/33041109/xcodebuild-no-applicable-devices-found-when-exporting-archive
# - https://gist.github.com/claybridges/cea5d4afd24eda268164
unset GEM_HOME
unset RUBYLIB
unset RUBYOPT
unset BUNDLE_BIN_PATH
unset _ORIGINAL_GEM_PATH
unset BUNDLE_GEMFILE

#
# Because of an RVM issue which conflicts with `xcodebuild`'s new
#  `-exportOptionsPlist` option
# link: https://github.com/bitrise-io/steps-xcode-archive/issues/13
command_exists () {
	command -v "$1" >/dev/null 2>&1 ;
}
if command_exists rvm ; then
	echo_warn "Applying RVM 'fix'"

	[[ -s "$HOME/.rvm/scripts/rvm" ]] && source "$HOME/.rvm/scripts/rvm"
	rvm use system
fi

tmp_dir=$(mktemp -d -t bitrise-xcarchive)

export_command="xcodebuild -exportArchive"
export_command="$export_command -archivePath \"${archive_path}\""

# It seems -exportOptionsPlist doesn't support the 'none' method, and
# an absense of an explicit method defaults to 'development', so we
# have to use the older, deprecated style in that case
if [[ "${export_method}" == "none" ]]; then
	export_command="$export_command -exportFormat APP"
	export_command="$export_command -exportPath \"${tmp_dir}/${scheme}.${export_format}\""
else
	if [ -z "${export_options_path}" ] ; then
		echo_info "Generating exportOptionsPlist..."

		gemfile_path="$THIS_SCRIPT_DIR/export-options/Gemfile"
		generate_osx_export_options_script_path="$THIS_SCRIPT_DIR/export-options/generate_osx_export_options.rb"
		export_options_path="${output_dir}/export_options.plist"

		BUNDLE_GEMFILE=$gemfile_path bundle install
		BUNDLE_GEMFILE=$gemfile_path bundle exec ruby $generate_osx_export_options_script_path \
			-o "${export_options_path}" \
			-a "${archive_path}" \
			-e "${export_method}"
	fi

	export_command="$export_command -exportOptionsPlist \"${export_options_path}\""
	export_command="$export_command -exportPath \"${tmp_dir}\""
fi

if [[ "${output_tool}" == "xcpretty" ]] ; then
	export_command="set -o pipefail && $export_command | xcpretty"
fi

echo_details "$ $export_command"
echo

eval $export_command
echo

# Searching for output file
exported_file_path=""
deploy_path=""

IFS=$'\n'
for a_file_path in $(find "$tmp_dir" -maxdepth 1 -mindepth 1 -name "*.${export_format}")
do
	filename=$( basename "${a_file_path}" )

	if [[ "${export_format}" == "app" ]] ; then
		dirname=$( dirname "${a_file_path}" )
		deploy_path="${output_dir}/${scheme}.app.zip"

		echo_details "zipping file: ${a_file_path} to ${deploy_path}"

		# cd into app parent to not to store full
		#  paths in the ZIP
		cd "${dirname}"
		zip_output=$(/usr/bin/zip -rTy "${deploy_path}" "${filename}")
		cd -
	else
		deploy_path="${output_dir}/${scheme}.pkg"

		mv "${a_file_path}" "${deploy_path}"
	fi

	if [[ -z "${exported_file_path}" ]] ; then
		exported_file_path="${deploy_path}"
	else
		echo_warn "More ${export_format} file found"
	fi
done
unset IFS

if [[ -z "${exported_file_path}" ]] ; then
	echo_fail "No exported file found"
fi

if [ ! -e "${exported_file_path}" ] ; then
	echo_fail "Failed to move APP to output dir"
fi

#
# Export output file
export BITRISE_EXPORTED_FILE_PATH="${exported_file_path}"
envman add --key BITRISE_EXPORTED_FILE_PATH --value "${exported_file_path}"
echo_done "The ${export_format} path is now available in the Environment Variable: $BITRISE_EXPORTED_FILE_PATH (value: \"$BITRISE_EXPORTED_FILE_PATH\")"

#
# dSYM handling
# get the .dSYM folders from the dSYMs archive folder
echo_info "Exporting dSym from generated Archive..."

archive_dsyms_folder="${archive_path}/dSYMs"
ls "${archive_dsyms_folder}"

app_dsym_regex='.*.app.dSYM'
app_dsym_paths=()
other_dsym_paths=()

IFS=$'\n'
for a_dsym in $(find "${archive_dsyms_folder}" -type d -name "*.dSYM") ; do
  if [[ $a_dsym =~ $app_dsym_regex ]] ; then
  	app_dsym_paths=(${app_dsym_paths[@]} "$a_dsym")
  else
  	other_dsym_paths=(${other_dsym_paths[@]} "$a_dsym")
  fi	
done
unset IFS

app_dsym_count=${#app_dsym_paths[@]}
other_dsym_count=${#other_dsym_paths[@]}

echo 
echo_details "app_dsym_count: $app_dsym_count"
echo_details "other_dsym_count: $other_dsym_count"

DSYM_PATH=""
if [[ "$is_export_all_dsyms" == "yes" ]] ; then
  tmp_dir="$(mktemp -d -t bitrise-dsym)/"

  dsym_paths=("${app_dsym_paths[@]}" "${other_dsym_paths[@]}")

  IFS=$'\n'
  for dsym_path in "${dsym_paths[@]}" ; do
	dsym_fold_name=$(basename "${dsym_path}" )

  	cp -r "${dsym_path}" "${tmp_dir}/${dsym_fold_name}"
  done
  unset IFS

  DSYM_PATH="${tmp_dir}"
else
  if [ ${app_dsym_count} -eq 1 ] ; then
    app_dsym_path="${app_dsym_paths[0]}"
	
    if [ -d "${app_dsym_path}" ] ; then
	  DSYM_PATH="${app_dsym_path}"
	else 
	  echo_warn "Found dSYM path is not a directory!"
	fi
  else
    if [ ${app_dsym_count} -eq 0 ] ; then
	  echo_warn "No dSYM found!"
	  echo_details "To generate debug symbols (dSYM) go to your Xcode Project's Settings - *Build Settings - Debug Information Format* and set it to *DWARF with dSYM File*."
	else
	  echo_warn "More than one dSYM found!"
	fi
  fi
fi

# Generate dSym zip
if [[ ! -z "${DSYM_PATH}" && -d "${DSYM_PATH}" ]] ; then
  echo_info "Generating zip for dSym..."

  dsym_parent_folder=$(dirname "${DSYM_PATH}")
  dsym_fold_name=$(basename "${DSYM_PATH}")
  # cd into dSYM parent to not to store full
  #  paths in the ZIP
  cd "${dsym_parent_folder}"
  zip_output=$(/usr/bin/zip -rTy "${dsym_zip_path}" "${dsym_fold_name}")
  cd -

	export BITRISE_DSYM_PATH="${dsym_zip_path}"
	envman add --key BITRISE_DSYM_PATH --value "${BITRISE_DSYM_PATH}"
	echo_done 'The dSYM path is now available in the Environment Variable: $BITRISE_DSYM_PATH'" (value: $BITRISE_DSYM_PATH)"
else
	echo_warn "No dSYM found (or not a directory: ${DSYM_PATH})"
fi

#
# Export *.xcarchive path
if [[ "$is_export_xcarchive_zip" == "yes" ]] ; then
	echo_info "Exporting the Archive..."

	xcarchive_parent_folder=$( dirname "${archive_path}" )
	xcarchive_fold_name=$( basename "${archive_path}" )
	xcarchive_zip_path="${output_dir}/${scheme}.xcarchive.zip"
	# cd into dSYM parent to not to store full
	#  paths in the ZIP
	cd "${xcarchive_parent_folder}"
	zip_output=$(/usr/bin/zip -rTy "${xcarchive_zip_path}" "${xcarchive_fold_name}")
	cd -

	export BITRISE_XCARCHIVE_PATH="${xcarchive_zip_path}"
	envman add --key BITRISE_XCARCHIVE_PATH --value "${BITRISE_XCARCHIVE_PATH}"
	echo_done 'The xcarchive path is now available in the Environment Variable: $BITRISE_XCARCHIVE_PATH'" (value: $BITRISE_XCARCHIVE_PATH)"
fi

exit 0
