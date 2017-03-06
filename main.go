package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"path/filepath"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-tools/go-xcode/exportoptions"
	"github.com/bitrise-tools/go-xcode/utility"
	"github.com/bitrise-tools/go-xcode/xcarchive"
	"github.com/bitrise-tools/go-xcode/xcodebuild"
	"github.com/bitrise-tools/go-xcode/xcpretty"
)

const (
	bitriseXcodeRawResultTextEnvKey = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"
	bitriseExportedFilePath         = "BITRISE_EXPORTED_FILE_PATH"
	bitriseDSYMDirPthEnvKey         = "BITRISE_DSYM_PATH"
	bitriseXCArchivePthEnvKey       = "BITRISE_XCARCHIVE_PATH"
	bitriseAppPthEnvKey             = "BITRISE_APP_PATH"
)

// ConfigsModel ...
type ConfigsModel struct {
	ExportMethod string

	ForceTeamID                       string
	ForceProvisioningProfileSpecifier string
	ForceProvisioningProfile          string
	ForceCodeSignIdentity             string
	CustomExportOptionsPlistContent   string

	OutputTool    string
	WorkDir       string
	ProjectPath   string
	Scheme        string
	Configuration string
	ArtifactName  string
	OutputDir     string
	IsCleanBuild  string

	IsExportXcarchiveZip string
	IsExportAllDsyms     string

	IsForceCodeSign   string
	ExportOptionsPath string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		ExportMethod: os.Getenv("export_method"),

		ForceTeamID:                       os.Getenv("force_team_id"),
		ForceProvisioningProfileSpecifier: os.Getenv("force_provisioning_profile_specifier"),
		ForceProvisioningProfile:          os.Getenv("force_provisioning_profile"),
		ForceCodeSignIdentity:             os.Getenv("force_code_sign_identity"),
		CustomExportOptionsPlistContent:   os.Getenv("custom_export_options_plist_content"),

		OutputTool:    os.Getenv("output_tool"),
		WorkDir:       os.Getenv("workdir"),
		ProjectPath:   os.Getenv("project_path"),
		Scheme:        os.Getenv("scheme"),
		Configuration: os.Getenv("configuration"),
		OutputDir:     os.Getenv("output_dir"),
		ArtifactName:  os.Getenv("artifact_name"),
		IsCleanBuild:  os.Getenv("is_clean_build"),

		IsExportXcarchiveZip: os.Getenv("is_export_xcarchive_zip"),
		IsExportAllDsyms:     os.Getenv("is_export_all_dsyms"),

		IsForceCodeSign:   os.Getenv("is_force_code_sign"),
		ExportOptionsPath: os.Getenv("export_options_path"),
	}
}

func (configs ConfigsModel) print() {
	fmt.Println()

	log.Infof("app/pkg export configs:")
	useCustomExportOptions := (configs.CustomExportOptionsPlistContent != "")
	if useCustomExportOptions {
		fmt.Println()
		log.Warnf("Ignoring the following options because CustomExportOptionsPlistContent provided:")
	}

	log.Printf("- ExportMethod: %s", configs.ExportMethod)

	if useCustomExportOptions {
		log.Warnf("----------")
	}

	log.Printf("- ForceTeamID: %s", configs.ForceTeamID)
	log.Printf("- ForceProvisioningProfileSpecifier: %s", configs.ForceProvisioningProfileSpecifier)
	log.Printf("- ForceProvisioningProfile: %s", configs.ForceProvisioningProfile)
	log.Printf("- ForceCodeSignIdentity: %s", configs.ForceCodeSignIdentity)
	log.Printf("- CustomExportOptionsPlistContent:")
	if configs.CustomExportOptionsPlistContent != "" {
		log.Printf(configs.CustomExportOptionsPlistContent)
	}
	log.Infof("xcodebuild configs:")
	log.Printf("- WorkDir: %s", configs.WorkDir)
	log.Printf("- ProjectPath: %s", configs.ProjectPath)
	log.Printf("- Scheme: %s", configs.Scheme)
	log.Printf("- Configuration: %s", configs.Configuration)
	log.Printf("- IsCleanBuild: %s", configs.IsCleanBuild)

	log.Infof("step output configs:")
	log.Printf("- OutputTool: %s", configs.OutputTool)
	log.Printf("- OutputDir: %s", configs.OutputDir)
	log.Printf("- ArtifactName: %s", configs.ArtifactName)
	log.Printf("- IsExportXcarchiveZip: %s", configs.IsExportXcarchiveZip)
	log.Printf("- IsExportAllDsyms: %s", configs.IsExportAllDsyms)

	log.Infof("DEPRECATED configs:")
	log.Printf("- IsForceCodeSign: %s", configs.IsForceCodeSign)
	log.Printf("- ExportOptionsPath: %s", configs.ExportOptionsPath)
}

//--------------------
// Functions
//--------------------
func validateRequiredInput(key, value string) error {
	if value == "" {
		return fmt.Errorf("Missing required input: %s", key)
	}
	return nil
}

func validateRequiredInputWithOptions(key, value string, options []string) error {
	if err := validateRequiredInput(key, value); err != nil {
		return err
	}

	found := false
	for _, option := range options {
		if option == value {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("Invalid input: (%s) value: (%s), valid options: %s", key, value, strings.Join(options, ", "))
	}

	return nil
}

func (configs ConfigsModel) validate() error {
	// required
	if err := validateRequiredInput("project_path", configs.ProjectPath); err != nil {
		return err
	}
	if exists, err := pathutil.IsDirExists(configs.ProjectPath); err != nil {
		return err
	} else if !exists {
		return errors.New("ProjectPath directory does not exists: %s")
	}

	if err := validateRequiredInput("output_dir", configs.OutputDir); err != nil {
		return err
	}
	if exists, err := pathutil.IsDirExists(configs.OutputDir); err != nil {
		return err
	} else if !exists {
		return errors.New("OutputDir directory does not exists: %s")
	}

	if err := validateRequiredInput("scheme", configs.Scheme); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions("output_tool", configs.OutputTool, []string{"xcpretty", "xcodebuild"}); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions("is_clean_build", configs.IsCleanBuild, []string{"yes", "no"}); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions("is_export_xcarchive_zip", configs.IsExportXcarchiveZip, []string{"yes", "no"}); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions("is_export_all_dsyms", configs.IsExportAllDsyms, []string{"yes", "no"}); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions("export_method", configs.ExportMethod, []string{"none", "app-store", "development", "developer-id"}); err != nil {
		return err
	}

	if err := validateRequiredInput("artifact_name", configs.ArtifactName); err != nil {
		return err
	}

	if configs.ExportOptionsPath != "" {
		fmt.Println()
		log.Warnf("ExportOptionsPath is deprecated!")
		log.Warnf("Use `custom_export_options_plist_content` instead.")
	}

	if configs.IsForceCodeSign != "no" {
		fmt.Println()
		log.Warnf("IsForceCodeSign is deprecated!")
		log.Warnf("Use `force_code_sign_identity` and `force_provisioning_profile_specifier/force_provisioning_profile` instead.")
	}

	return nil
}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}

// ExportEnvironmentWithEnvman ...
func ExportEnvironmentWithEnvman(keyStr, valueStr string) error {
	return command.New("envman", "add", "--key", keyStr).SetStdin(strings.NewReader(valueStr)).Run()
}

// GetXcprettyVersion ...
func GetXcprettyVersion() (string, error) {
	cmd := command.New("xcpretty", "-version")
	return cmd.RunAndReturnTrimmedCombinedOutput()
}

// ExportOutputFile ...
func ExportOutputFile(sourcePth, destinationPth, envKey string) error {
	if sourcePth != destinationPth {
		if err := command.CopyFile(sourcePth, destinationPth); err != nil {
			return err
		}
	}

	return ExportEnvironmentWithEnvman(envKey, destinationPth)
}

// ExportOutputFileContent ...
func ExportOutputFileContent(content, destinationPth, envKey string) error {
	if err := fileutil.WriteStringToFile(destinationPth, content); err != nil {
		return err
	}

	return ExportOutputFile(destinationPth, destinationPth, envKey)
}

func zip(sourceDir, destinationZipPth string) error {
	parentDir := filepath.Dir(sourceDir)
	dirName := filepath.Base(sourceDir)
	cmd := command.New("/usr/bin/zip", "-rTy", destinationZipPth, dirName)
	cmd.SetDir(parentDir)
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to zip dir: %s, output: %s, error: %s", sourceDir, out, err)
	}

	return nil
}

// ExportOutputDirAsZip ...
func ExportOutputDirAsZip(sourceDirPth, destinationPth, envKey string) error {
	tmpDir, err := pathutil.NormalizedOSTempDirPath("__export_tmp_dir__")
	if err != nil {
		return err
	}

	base := filepath.Base(sourceDirPth)
	tmpZipFilePth := filepath.Join(tmpDir, base+".zip")

	if err := zip(sourceDirPth, tmpZipFilePth); err != nil {
		return err
	}

	return ExportOutputFile(tmpZipFilePth, destinationPth, envKey)
}

// ExportOutputDir ...
func ExportOutputDir(sourceDirPth, destinationDirPth, envKey string) error {
	if sourceDirPth != destinationDirPth {
		if err := command.CopyDir(sourceDirPth, destinationDirPth, true); err != nil {
			return err
		}
	}

	return ExportEnvironmentWithEnvman(envKey, destinationDirPth)
}

//--------------------
// Main
//--------------------

func main() {
	configs := createConfigsModelFromEnvs()
	configs.print()
	if err := configs.validate(); err != nil {
		failf("Issue with input: %s", err)
	}

	log.Infof("step determined configs:")

	// Detect Xcode major version
	xcodebuildVersion, err := utility.GetXcodeVersion()
	if err != nil {
		failf("Failed to get the version of xcodebuild! Error: %s", err)
	}
	log.Printf("- xcodebuild_version: %s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)
	if configs.ExportOptionsPath != "" && xcodebuildVersion.MajorVersion == 6 {
		log.Warnf("Xcode major version: 6, export_options_path only used if xcode major version > 6")
		configs.ExportOptionsPath = ""
	}

	// Detect xcpretty version
	if configs.OutputTool == "xcpretty" {
		xcprettyVersion, err := GetXcprettyVersion()
		if err != nil {
			failf("Failed to get the xcpretty version! Error: %s", err)
		} else {
			log.Printf("- xcpretty_version: %s", xcprettyVersion)
		}
	}

	// Validation CustomExportOptionsPlistContent
	if configs.CustomExportOptionsPlistContent != "" &&
		xcodebuildVersion.MajorVersion < 7 {
		log.Warnf("CustomExportOptionsPlistContent is set, but CustomExportOptionsPlistContent only used if xcodeMajorVersion > 6")
		configs.CustomExportOptionsPlistContent = ""
	}

	if configs.ForceProvisioningProfileSpecifier != "" &&
		xcodebuildVersion.MajorVersion < 8 {
		log.Warnf("ForceProvisioningProfileSpecifier is set, but ForceProvisioningProfileSpecifier only used if xcodeMajorVersion > 7")
		configs.ForceProvisioningProfileSpecifier = ""
	}

	if configs.ForceTeamID == "" &&
		xcodebuildVersion.MajorVersion < 8 {
		log.Warnf("ForceTeamID is set, but ForceTeamID only used if xcodeMajorVersion > 7")
		configs.ForceTeamID = ""
	}

	if configs.ForceProvisioningProfileSpecifier != "" &&
		configs.ForceProvisioningProfile != "" {
		log.Warnf("both ForceProvisioningProfileSpecifier and ForceProvisioningProfile are set, using ForceProvisioningProfileSpecifier")
		configs.ForceProvisioningProfile = ""
	}

	// Project-or-Workspace flag
	action := ""
	if strings.HasSuffix(configs.ProjectPath, ".xcodeproj") {
		action = "-project"
	} else if strings.HasSuffix(configs.ProjectPath, ".xcworkspace") {
		action = "-workspace"
	} else {
		failf("Invalid project file (%s), extension should be (.xcodeproj/.xcworkspace)", configs.ProjectPath)
	}

	log.Printf("- action: %s", action)

	// export format
	exportFormat := "app"
	if configs.ExportMethod == "app-store" {
		exportFormat = "pkg"
	}
	log.Printf("- export_format: %s", exportFormat)

	fmt.Println()

	// abs out dir pth
	absOutputDir, err := pathutil.AbsPath(configs.OutputDir)
	if err != nil {
		failf("Failed to expand OutputDir (%s), error: %s", configs.OutputDir, err)
	}
	configs.OutputDir = absOutputDir

	if exist, err := pathutil.IsPathExists(configs.OutputDir); err != nil {
		failf("Failed to check if OutputDir exist, error: %s", err)
	} else if !exist {
		if err := os.MkdirAll(configs.OutputDir, 0777); err != nil {
			failf("Failed to create OutputDir (%s), error: %s", configs.OutputDir, err)
		}
	}

	// output files
	archiveTempDir, err := pathutil.NormalizedOSTempDirPath("bitrise-xcarchive")
	if err != nil {
		failf("Failed to create archive tmp dir, error: %s", err)
	}

	archivePath := filepath.Join(archiveTempDir, configs.ArtifactName+".xcarchive")
	log.Printf("- archivePath: %s", archivePath)

	archiveZipPath := filepath.Join(configs.OutputDir, configs.ArtifactName+".xcarchive.zip")
	log.Printf("- archiveZipPath: %s", archiveZipPath)

	exportOptionsPath := filepath.Join(configs.OutputDir, "export_options.plist")
	log.Printf("- exportOptionsPath: %s", exportOptionsPath)

	filePath := filepath.Join(configs.OutputDir, configs.ArtifactName+"."+exportFormat)
	log.Printf("- filePath: %s", filePath)

	dsymZipPath := filepath.Join(configs.OutputDir, configs.ArtifactName+".dSYM.zip")
	log.Printf("- dsymZipPath: %s", dsymZipPath)

	rawXcodebuildOutputLogPath := filepath.Join(configs.OutputDir, "raw-xcodebuild-output.log")
	log.Printf("- rawXcodebuildOutputLogPath: %s", rawXcodebuildOutputLogPath)

	fmt.Println()

	// clean-up
	filesToCleanup := []string{
		filePath,
		dsymZipPath,
		rawXcodebuildOutputLogPath,
		archiveZipPath,
		exportOptionsPath,
	}

	for _, pth := range filesToCleanup {
		if exist, err := pathutil.IsPathExists(pth); err != nil {
			failf("Failed to check if path (%s) exist, error: %s", pth, err)
		} else if exist {
			if err := os.RemoveAll(pth); err != nil {
				failf("Failed to remove path (%s), error: %s", pth, err)
			}
		}
	}

	//
	// Create the Archive with Xcode Command Line tools
	log.Infof("Create archive ...")
	fmt.Println()

	isWorkspace := false
	ext := filepath.Ext(configs.ProjectPath)
	if ext == ".xcodeproj" {
		isWorkspace = false
	} else if ext == ".xcworkspace" {
		isWorkspace = true
	} else {
		failf("Project file extension should be .xcodeproj or .xcworkspace, but got: %s", ext)
	}

	archiveCmd := xcodebuild.NewArchiveCommand(configs.ProjectPath, isWorkspace)
	archiveCmd.SetScheme(configs.Scheme)
	archiveCmd.SetConfiguration(configs.Configuration)

	if configs.ForceTeamID != "" {
		log.Printf("Forcing Development Team: %s", configs.ForceTeamID)
		archiveCmd.SetForceDevelopmentTeam(configs.ForceTeamID)
	}
	if configs.ForceProvisioningProfileSpecifier != "" {
		log.Printf("Forcing Provisioning Profile Specifier: %s", configs.ForceProvisioningProfileSpecifier)
		archiveCmd.SetForceProvisioningProfileSpecifier(configs.ForceProvisioningProfileSpecifier)
	}
	if configs.ForceProvisioningProfile != "" {
		log.Printf("Forcing Provisioning Profile: %s", configs.ForceProvisioningProfile)
		archiveCmd.SetForceProvisioningProfile(configs.ForceProvisioningProfile)
	}
	if configs.ForceCodeSignIdentity != "" {
		log.Printf("Forcing Code Signing Identity: %s", configs.ForceCodeSignIdentity)
		archiveCmd.SetForceCodeSignIdentity(configs.ForceCodeSignIdentity)
	}

	if configs.IsCleanBuild == "yes" {
		archiveCmd.SetCustomBuildAction("clean")
	}

	archiveCmd.SetArchivePath(archivePath)

	if configs.OutputTool == "xcpretty" {
		xcprettyCmd := xcpretty.New(archiveCmd)

		log.Doneft("$ %s", xcprettyCmd.PrintableCmd())
		fmt.Println()

		if rawXcodebuildOut, err := xcprettyCmd.Run(); err != nil {
			if err := ExportOutputFileContent(rawXcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
				log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
			} else {
				log.Warnf(`If you can't find the reason of the error in the log, please check the raw-xcodebuild-output.log
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable`)
			}

			failf("Archive failed, error: %s", err)
		}
	} else {
		log.Doneft("$ %s", archiveCmd.PrintableCmd())
		fmt.Println()

		if err := archiveCmd.Run(); err != nil {
			failf("Archive failed, error: %s", err)
		}
	}

	// Ensure xcarchive exists
	if exist, err := pathutil.IsPathExists(archivePath); err != nil {
		failf("Failed to check if archive exist, error: %s", err)
	} else if !exist {
		failf("No archive generated at: %s", archivePath)
	}

	// Exporting xcarchive
	fmt.Println()
	log.Infof("Exporting xcarchive ...")
	fmt.Println()

	if configs.IsExportXcarchiveZip == "yes" {
		if err := ExportOutputDirAsZip(archivePath, archiveZipPath, bitriseXCArchivePthEnvKey); err != nil {
			failf("Failed to export %s, error: %s", bitriseXCArchivePthEnvKey, err)
		}

		log.Donef("The xcarchive zip path is now available in the Environment Variable: %s (value: %s)", bitriseXCArchivePthEnvKey, archiveZipPath)
	}

	fmt.Println()

	// Export APP from generated archive
	log.Infof("Exporting APP from generated Archive ...")

	envsToUnset := []string{"GEM_HOME", "GEM_PATH", "RUBYLIB", "RUBYOPT", "BUNDLE_BIN_PATH", "_ORIGINAL_GEM_PATH", "BUNDLE_GEMFILE"}
	for _, key := range envsToUnset {
		if err := os.Unsetenv(key); err != nil {
			failf("Failed to unset (%s), error: %s", key, err)
		}
	}

	// Legacy
	if configs.ExportMethod == "none" {
		log.Printf("Export a copy of the application without re-signing...")
		fmt.Println()

		embeddedAppPattern := filepath.Join(archivePath, "Products", "Applications", "*.app")
		matches, err := filepath.Glob(embeddedAppPattern)
		if err != nil {
			failf("Failed to find embedded app with pattern: %s, error: %s", embeddedAppPattern, err)
		}

		if len(matches) == 0 {
			failf("No embedded app found with pattern: %s", embeddedAppPattern)
		} else if len(matches) > 1 {
			failf("Multiple embedded app found with pattern: %s", embeddedAppPattern)
		}

		embeddedAppPath := matches[0]
		appPath := filepath.Join(configs.OutputDir, configs.ArtifactName+".app")

		if err := ExportOutputFile(embeddedAppPath, appPath, bitriseAppPthEnvKey); err != nil {
			failf("Failed to export %s, error: %s", bitriseAppPthEnvKey, err)
		}

		filePath = filePath + ".zip"
		if err := ExportOutputDirAsZip(embeddedAppPath, filePath, bitriseExportedFilePath); err != nil {
			failf("Failed to export %s, error: %s", bitriseExportedFilePath, err)
		}

		log.Donef("The app path is now available in the Environment Variable: %s (value: %s)", bitriseExportedFilePath, filePath)
	} else {
		// export using exportOptions
		log.Printf("Export using exportOptions...")
		fmt.Println()

		exportTmpDir, err := pathutil.NormalizedOSTempDirPath("__export__")
		if err != nil {
			failf("Failed to create export tmp dir, error: %s", err)
		}

		exportCmd := xcodebuild.NewExportCommand()
		exportCmd.SetArchivePath(archivePath)
		exportCmd.SetExportDir(exportTmpDir)

		exportMethod, err := exportoptions.ParseMethod(configs.ExportMethod)
		if err != nil {
			failf("Failed to parse export method, error: %s", err)
		}

		var exportOpts exportoptions.ExportOptions
		if exportMethod == exportoptions.MethodAppStore {
			exportOpts = exportoptions.NewAppStoreOptions()
		} else {
			exportOpts = exportoptions.NewNonAppStoreOptions(exportMethod)
		}

		log.Printf("generated export options content:")
		fmt.Println()
		fmt.Println(exportOpts.String())

		if err = exportOpts.WriteToFile(exportOptionsPath); err != nil {
			failf("Failed to write export options to file, error: %s", err)
		}

		exportCmd.SetExportOptionsPlist(exportOptionsPath)

		if configs.OutputTool == "xcpretty" {
			xcprettyCmd := xcpretty.New(exportCmd)

			log.Donef("$ %s", xcprettyCmd.PrintableCmd())
			fmt.Println()

			if rawXcodebuildOut, err := xcprettyCmd.Run(); err != nil {
				if err := ExportOutputFileContent(rawXcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
					log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
				} else {
					log.Warnf(`If you can't find the reason of the error in the log, please check the raw-xcodebuild-output.log
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable`)
				}

				failf("Archive failed, error: %s", err)
			}
		} else {
			log.Donef("$ %s", exportCmd.PrintableCmd())
			fmt.Println()

			if err := exportCmd.Run(); err != nil {
				failf("Export failed, error: %s", err)
			}
		}

		// find exported app
		pattern := filepath.Join(exportTmpDir, "*."+exportFormat)
		apps, err := filepath.Glob(pattern)
		if err != nil {
			failf("Failed to find app, with pattern: %s, error: %s", pattern, err)
		}

		if len(apps) > 0 {
			if exportFormat == "pkg" {
				if err := ExportOutputFile(apps[0], filePath, bitriseExportedFilePath); err != nil {
					failf("Failed to export %s, error: %s", bitriseExportedFilePath, err)
				}
			} else {
				if err := ExportEnvironmentWithEnvman(bitriseAppPthEnvKey, filePath); err != nil {
					failf("Failed to export %s, error: %s", bitriseAppPthEnvKey, err)
				}
				filePath = filePath + ".zip"
				if err := ExportOutputDirAsZip(apps[0], filePath, bitriseExportedFilePath); err != nil {
					failf("Failed to export %s, error: %s", bitriseExportedFilePath, err)
				}
			}

			fmt.Println()
			log.Donef("The app path is now available in the Environment Variable: %s (value: %s)", bitriseExportedFilePath, filePath)
		}
	}

	// Export .dSYM files
	fmt.Println()
	log.Infof("Exporting dSYM files ...")
	fmt.Println()

	appDSYM, frameworkDSYMs, err := xcarchive.FindDSYMs(archivePath)
	if err != nil {
		failf("Failed to export dsyms, error: %s", err)
	}

	dsymDir, err := pathutil.NormalizedOSTempDirPath("__dsyms__")
	if err != nil {
		failf("Failed to create tmp dir, error: %s", err)
	}

	if err := command.CopyDir(appDSYM, dsymDir, false); err != nil {
		failf("Failed to copy (%s) -> (%s), error: %s", appDSYM, dsymDir, err)
	}

	if configs.IsExportAllDsyms == "yes" {
		for _, dsym := range frameworkDSYMs {
			if err := command.CopyDir(dsym, dsymDir, false); err != nil {
				failf("Failed to copy (%s) -> (%s), error: %s", dsym, dsymDir, err)
			}
		}
	}

	if err := ExportOutputDirAsZip(dsymDir, dsymZipPath, bitriseDSYMDirPthEnvKey); err != nil {
		failf("Failed to export %s, error: %s", bitriseDSYMDirPthEnvKey, err)
	}

	log.Donef("The dSYM dir path is now available in the Environment Variable: %s (value: %s)", bitriseDSYMDirPthEnvKey, dsymZipPath)
}
