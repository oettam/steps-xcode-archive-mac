#!/bin/bash

THIS_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

set -e

#
# Detect Xcode major version
xcode_major_version=""
major_version_regex="Xcode ([0-9]).[0-9]"
out=$(xcodebuild -version)
if [[ "${out}" =~ ${major_version_regex} ]] ; then
	xcode_major_version="${BASH_REMATCH[1]}"
fi

if [ ! "${xcode_major_version}" == "7" ] && [ ! "${xcode_major_version}" == "6" ] ; then
	echo "Invalid xcode major version: ${xcode_major_version}"
	exit 1
fi

echo "(i) xcode_major_version: ${xcode_major_version}"


#
# Required parameters
if [ -z "${project_path}" ] ; then
	echo "[!] Missing required input: project_path"
	exit 1
fi

if [ -z "${scheme}" ] ; then
	echo "[!] Missing required input: scheme"
	exit 1
fi

if [ -z "${output_dir}" ] ; then
	echo "[!] Missing required input: output_dir"
	exit 1
fi

if [[ "${output_tool}" != "xcpretty" && "${output_tool}" != "xcodebuild" ]] ; then
	echo "[!] Invalid output_tool: ${output_tool}"
	exit 1
fi

set +e

if [[ "${output_tool}" == "xcpretty" ]] ; then
	xcpretty_version=$(xcpretty --version)
	exit_code=$?
	if [[ $exit_code != 0 || -z $xcpretty_version ]] ; then
		echo
		echo " (!) xcpretty is not installed"
		echo "     For xcpretty installation see: 'https://github.com/supermarin/xcpretty',"
		echo "     or use 'xcodebuild' as 'output_tool'."
		echo
		exit 1
	fi
fi

set -e

#
# Project-or-Workspace flag
if [[ "${project_path}" == *".xcodeproj" ]]; then
	CONFIG_xcode_project_action="-project"
elif [[ "${project_path}" == *".xcworkspace" ]]; then
	CONFIG_xcode_project_action="-workspace"
else
	echo "Failed to get valid project file (invalid project file): ${project_path}"
	exit 1
fi

# abs out dir pth
mkdir -p "${output_dir}"
cd "${output_dir}"
output_dir="$(pwd)"
cd -

archive_tmp_dir=$(mktemp -d -t bitrise-xcarchive)
archive_path="${archive_tmp_dir}/${scheme}.xcarchive"
app_path="${output_dir}/${scheme}.app"
dsym_zip_path="${output_dir}/${scheme}.dSYM.zip"

if [ -z "${workdir}" ] ; then
	workdir="$(pwd)"
fi

#
# Print configs
echo
echo "========== Configs =========="
echo " * CONFIG_xcode_project_action: ${CONFIG_xcode_project_action}"
echo " * output_tool: ${output_tool}"
echo " * project_path: ${project_path}"
echo " * scheme: ${scheme}"
echo " * workdir: ${workdir}"
echo " * output_dir: ${output_dir}"
echo " * archive_path: ${archive_path}"
echo " * app_path: ${app_path}"
echo " * dsym_zip_path: ${dsym_zip_path}"
echo " * is_force_code_sign: ${is_force_code_sign}"
echo " * is_clean_build: ${is_clean_build}"
echo " * configuration: ${configuration}"

if [ ! -z "${workdir}" ] ; then
	echo
	echo " -> Switching to working directory: ${workdir}"
	cd "${workdir}"
fi

xcode_configuration=''
if [ ! -z "${configuration}" ] ; then
	xcode_configuration="-configuration ${configuration}"
fi

clean_build_param=''
if [[ "${is_clean_build}" == "yes" ]] ; then
	clean_build_param='clean'
fi

#
# Cleanup function
function finalcleanup {
	local fail_msg="$1"

	echo "-> finalcleanup"

	if [ ! -z "${fail_msg}" ] ; then
		echo " [!] ERROR: ${fail_msg}"
		exit 1
	fi
}

#
# Main

#
# Bit of cleanup
if [ -f "${app_path}" ] ; then
	echo " (!) App at path (${app_path}) already exists - removing it"
	rm "${app_path}"
fi

echo
echo
echo "=> Create the Archive ..."

#
# Create the Archive with Xcode Command Line tools
archive_cmd="xcodebuild ${CONFIG_xcode_project_action} \"${project_path}\""
archive_cmd="$archive_cmd -scheme \"${scheme}\" ${xcode_configuration}"
archive_cmd="$archive_cmd ${clean_build_param} archive -archivePath \"${archive_path}\""

if [[ "${is_force_code_sign}" == "yes" ]] ; then
	echo " (!) Using Force Code Signing mode!"

	archive_cmd="$archive_cmd PROVISIONING_PROFILE=\"${BITRISE_PROVISIONING_PROFILE_ID}\""
	archive_cmd="$archive_cmd CODE_SIGN_IDENTITY=\"${BITRISE_CODE_SIGN_IDENTITY}\""
fi

if [[ "${output_tool}" == "xcpretty" ]] ; then
	archive_cmd="set -o pipefail && $archive_cmd | xcpretty"
fi

echo
echo "archive command:"
echo "$archive_cmd"
echo

eval $archive_cmd

echo
echo
echo "=> Exporting app from generated Archive ..."
echo

echo " (i) The archive path is now available at: ${archive_path}"
envman add --key BITRISE_ARCHIVE_PATH --value "${archive_path}"
echo ' (i) The archive path is now available in the Environment Variable: $BITRISE_ARCHIVE_PATH'

export_command="xcodebuild -exportArchive"

if [[ "${xcode_major_version}" == "6" ]] ; then
	#
	# Get the name of the profile which was used for creating the archive
	# --> Search for embedded.provisionprofile in the xcarchive.
	#     It should contain a .app folder in the xcarchive folder
	#     under the Products/Applications folder
	embedded_prov_path=""

	IFS=$'\n'
	for a_emb_path in $(find "${archive_path}/Products/Applications" -type f -maxdepth 3 -ipath "*/embedded.provisionprofile")
	do
		echo " * embedded.provisionprofile: ${a_emb_path}"
		if [ ! -z "${embedded_prov_path}" ] ; then
			finalcleanup "More than one \`embedded.provisionprofile\` found in \`${archive_path}/Products/Applications/*.app\`"
			exit 1
		fi
		embedded_prov_path="${a_emb_path}"
	done
	unset IFS

	if [ -z "${embedded_prov_path}" ] ; then
		finalcleanup "No \`embedded.provisionprofile\` found in \`${archive_path}/Products/Applications/*.app\`"
		exit 1
	fi

	#
	# We have the provisionprofile file - let's get the Profile name from it
	profile_name=`/usr/libexec/PlistBuddy -c 'Print :Name' /dev/stdin <<< $(security cms -D -i "${embedded_prov_path}")`
	if [ $? -ne 0 ] ; then
		finalcleanup "Missing embedded provisionprofile in xcarchive"
	fi

	echo " (i) Found Profile Name for signing: ${profile_name}"

	#
	# Use the Provisioning Profile name to export the APP or PKG
	export_command="$export_command -exportFormat \"${export_format}\""
	export_command="$export_command -archivePath \"${archive_path}\""
	export_command="$export_command -exportPath \"${app_path}\""
	export_command="$export_command -exportProvisioningProfile \"${profile_name}\""

	if [[ "${output_tool}" == "xcpretty" ]] ; then
		export_command="set -o pipefail && $export_command | xcpretty"
	fi

	echo
	echo "export command:"
	echo "$export_command"
	echo

	eval $export_command
else
	echo " (i) Using Xcode 7 'exportOptionsPlist' option"

	if [ -z "${export_options_path}" ] ; then
		export_options_path="${output_dir}/export_options.plist"
		curr_pwd="$(pwd)"
		cd "${THIS_SCRIPT_DIR}"
		bundle install
		bundle exec ruby "./generate_export_options.rb" \
			-o "${export_options_path}" \
			-a "${archive_path}" \
			-e "${export_method}"
		cd "${curr_pwd}"
	fi

	#
	# Because of an RVM issue which conflicts with `xcodebuild`'s new
	#  `-exportOptionsPlist` option
	# link: https://github.com/bitrise-io/steps-xcode-archive/issues/13
	command_exists () {
		command -v "$1" >/dev/null 2>&1 ;
	}
	if command_exists rvm ; then
		set +x
		echo "=> Applying RVM 'fix'"
		[[ -s "$HOME/.rvm/scripts/rvm" ]] && source "$HOME/.rvm/scripts/rvm"
		rvm use system
	fi

	tmp_dir=$(mktemp -d -t bitrise-xcarchive)

	export_command="$export_command -archivePath \"${archive_path}\""
	export_command="$export_command -exportPath \"${tmp_dir}/${scheme}\""
	export_command="$export_command -exportOptionsPlist \"${export_options_path}\""

	if [[ "${output_tool}" == "xcpretty" ]] ; then
		export_command="set -o pipefail && $export_command | xcpretty"
	fi

	echo
	echo "export command:"
	echo "$export_command"
	echo
	eval $export_command

	# Searching for app
	exported_app_path=""
	IFS=$'\n'
	for a_file_path in $(find "$tmp_dir" -maxdepth 1 -mindepth 1 -type d)
	do
		filename=$(basename "$a_file_path")
		echo " -> moving file: ${a_file_path} to ${output_dir}"

		mv "${a_file_path}" "${output_dir}"

		regex=".*.app"
		if [[ "${filename}" =~ $regex ]] ; then
			if [[ -z "${exported_app_path}" ]] ; then
				exported_app_path="${output_dir}/${filename}"
			else
				echo " (!) More app file found"
			fi
		fi
	done
	unset IFS

	if [[ -z "${exported_app_path}" ]] ; then
		echo " (!) No app file found"
		exit 1
	fi

	if [ ! -e "${exported_app_path}" ] ; then
		echo " (!) Failed to move app to output dir"
		exit 1
	fi

	app_path="${exported_app_path}"
fi

#
# Export *.app path
echo " (i) The APP is now available at: ${app_path}"
envman add --key BITRISE_EXPORTED_FILE_PATH --value "${app_path}"
echo ' (i) The APP path is now available in the Environment Variable: $BITRISE_EXPORTED_FILE_PATH'

#
# dSYM handling
# get the .app.dSYM folders from the dSYMs archive folder
archive_dsyms_folder="${archive_path}/dSYMs"
ls "${archive_dsyms_folder}"
app_dsym_count=0
app_dsym_path=""

IFS=$'\n'
for a_app_dsym in $(find "${archive_dsyms_folder}" -type d -name "*.app.dSYM") ; do
  echo " (i) .app.dSYM found: ${a_app_dsym}"
  app_dsym_count=$[app_dsym_count + 1]
  app_dsym_path="${a_app_dsym}"
  echo " (i) app_dsym_count: $app_dsym_count"
done
unset IFS

echo " (i) Found dSYM count: ${app_dsym_count}"
if [ ${app_dsym_count} -eq 1 ] ; then
  echo "* dSYM found at: ${app_dsym_path}"
  if [ -d "${app_dsym_path}" ] ; then
    export DSYM_PATH="${app_dsym_path}"
  else
    echo "* (i) *Found dSYM path is not a directory!*"
  fi
else
  if [ ${app_dsym_count} -eq 0 ] ; then
    echo "* (i) **No dSYM found!** To generate debug symbols (dSYM) go to your Xcode Project's Settings - *Build Settings - Debug Information Format* and set it to *DWARF with dSYM File*."
  else
    echo "* (i) *More than one dSYM found!*"
  fi
fi

# Generate dSym zip
if [[ ! -z "${DSYM_PATH}" && -d "${DSYM_PATH}" ]] ; then
  echo "Generating zip for dSym"

  dsym_parent_folder=$( dirname "${DSYM_PATH}" )
  dsym_fold_name=$( basename "${DSYM_PATH}" )
  # cd into dSYM parent to not to store full
  # paths in the ZIP
  cd "${dsym_parent_folder}"
  /usr/bin/zip -rTy \
    "${dsym_zip_path}" \
    "${dsym_fold_name}"

	echo " (i) The dSYM is now available at: ${dsym_zip_path}"
	envman add --key BITRISE_DSYM_PATH --value "${dsym_zip_path}"
	echo ' (i) The dSYM path is now available in the Environment Variable: $BITRISE_DSYM_PATH'
else
	echo " (!) No dSYM found (or not a directory: ${DSYM_PATH})"
fi

exit 0
