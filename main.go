package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-tools/go-xcode/certificateutil"
	"github.com/bitrise-tools/go-xcode/export"
	"github.com/bitrise-tools/go-xcode/profileutil"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-tools/go-steputils/input"
	"github.com/bitrise-tools/go-steputils/output"
	"github.com/bitrise-tools/go-steputils/tools"
	"github.com/bitrise-tools/go-xcode/exportoptions"
	"github.com/bitrise-tools/go-xcode/utility"
	"github.com/bitrise-tools/go-xcode/xcarchive"
	"github.com/bitrise-tools/go-xcode/xcodebuild"
	"github.com/bitrise-tools/go-xcode/xcpretty"
)

const (
	bitriseXcodeRawResultTextEnvKey     = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"
	bitriseExportedFilePath             = "BITRISE_EXPORTED_FILE_PATH"
	bitriseDSYMDirPthEnvKey             = "BITRISE_DSYM_PATH"
	bitriseXCArchivePthEnvKey           = "BITRISE_XCARCHIVE_PATH"
	bitriseXCArchiveDirPthEnvKey        = "BITRISE_MACOS_XCARCHIVE_PATH"
	bitriseAppPthEnvKey                 = "BITRISE_APP_PATH"
	bitriseIDEDistributionLogsPthEnvKey = "BITRISE_IDEDISTRIBUTION_LOGS_PATH"
)

// ConfigsModel ...
type ConfigsModel struct {
	ExportMethod                    string
	CustomExportOptionsPlistContent string

	ProjectPath   string
	Scheme        string
	Configuration string
	IsCleanBuild  string
	WorkDir       string

	ForceTeamID                       string
	ForceCodeSignIdentity             string
	ForceProvisioningProfileSpecifier string
	ForceProvisioningProfile          string

	OutputTool           string
	OutputDir            string
	ArtifactName         string
	IsExportXcarchiveZip string
	IsExportAllDsyms     string
	VerboseLog           string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		ExportMethod:                    os.Getenv("export_method"),
		CustomExportOptionsPlistContent: os.Getenv("custom_export_options_plist_content"),

		ProjectPath:   os.Getenv("project_path"),
		Scheme:        os.Getenv("scheme"),
		Configuration: os.Getenv("configuration"),
		IsCleanBuild:  os.Getenv("is_clean_build"),
		WorkDir:       os.Getenv("workdir"),

		ForceTeamID:                       os.Getenv("force_team_id"),
		ForceCodeSignIdentity:             os.Getenv("force_code_sign_identity"),
		ForceProvisioningProfileSpecifier: os.Getenv("force_provisioning_profile_specifier"),
		ForceProvisioningProfile:          os.Getenv("force_provisioning_profile"),

		OutputTool:           os.Getenv("output_tool"),
		OutputDir:            os.Getenv("output_dir"),
		ArtifactName:         os.Getenv("artifact_name"),
		IsExportXcarchiveZip: os.Getenv("is_export_xcarchive_zip"),
		IsExportAllDsyms:     os.Getenv("is_export_all_dsyms"),
		VerboseLog:           os.Getenv("verbose_log"),
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
	log.Printf("- CustomExportOptionsPlistContent:")
	if configs.CustomExportOptionsPlistContent != "" {
		log.Printf(configs.CustomExportOptionsPlistContent)
	}

	if useCustomExportOptions {
		log.Warnf("----------")
	}

	log.Infof("xcodebuild configs:")
	log.Printf("- ProjectPath: %s", configs.ProjectPath)
	log.Printf("- Scheme: %s", configs.Scheme)
	log.Printf("- Configuration: %s", configs.Configuration)
	log.Printf("- IsCleanBuild: %s", configs.IsCleanBuild)
	log.Printf("- WorkDir: %s", configs.WorkDir)

	log.Infof("force archive codesign settings:")
	log.Printf("- ForceTeamID: %s", configs.ForceTeamID)
	log.Printf("- ForceCodeSignIdentity: %s", configs.ForceCodeSignIdentity)
	log.Printf("- ForceProvisioningProfileSpecifier: %s", configs.ForceProvisioningProfileSpecifier)
	log.Printf("- ForceProvisioningProfile: %s", configs.ForceProvisioningProfile)

	log.Infof("step output configs:")
	log.Printf("- OutputTool: %s", configs.OutputTool)
	log.Printf("- OutputDir: %s", configs.OutputDir)
	log.Printf("- ArtifactName: %s", configs.ArtifactName)
	log.Printf("- IsExportXcarchiveZip: %s", configs.IsExportXcarchiveZip)
	log.Printf("- IsExportAllDsyms: %s", configs.IsExportAllDsyms)
	log.Printf("- VerboseLog: %s", configs.VerboseLog)
}

func (configs ConfigsModel) validate() error {
	if err := input.ValidateIfPathExists(configs.ProjectPath); err != nil {
		return fmt.Errorf("ProjectPath - %s", err)
	}

	if err := input.ValidateIfPathExists(configs.OutputDir); err != nil {
		return fmt.Errorf("OutputDir - %s", err)
	}

	if err := input.ValidateIfNotEmpty(configs.Scheme); err != nil {
		return fmt.Errorf("Scheme - %s", err)
	}

	if err := input.ValidateWithOptions(configs.OutputTool, "xcpretty", "xcodebuild"); err != nil {
		return fmt.Errorf("OutputTool - %s", err)
	}

	if err := input.ValidateWithOptions(configs.IsCleanBuild, "yes", "no"); err != nil {
		return fmt.Errorf("IsCleanBuild - %s", err)
	}

	if err := input.ValidateWithOptions(configs.IsExportXcarchiveZip, "yes", "no"); err != nil {
		return fmt.Errorf("IsExportXcarchiveZip - %s", err)
	}

	if err := input.ValidateWithOptions(configs.IsExportAllDsyms, "yes", "no"); err != nil {
		return fmt.Errorf("IsExportAllDsyms - %s", err)
	}

	if err := input.ValidateWithOptions(configs.ExportMethod, "none", "app-store", "development", "developer-id"); err != nil {
		return fmt.Errorf("ExportMethod - %s", err)
	}

	if err := input.ValidateIfNotEmpty(configs.ArtifactName); err != nil {
		return fmt.Errorf("ArtifactName - %s", err)
	}

	return nil
}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}

func getXcprettyVersion() (string, error) {
	cmd := command.New("xcpretty", "-version")
	return cmd.RunAndReturnTrimmedCombinedOutput()
}

func findIDEDistrubutionLogsPath(output string) (string, error) {
	pattern := `IDEDistribution: -\[IDEDistributionLogging _createLoggingBundleAtPath:\]: Created bundle at path '(?P<log_path>.*)'`
	re := regexp.MustCompile(pattern)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if match := re.FindStringSubmatch(line); len(match) == 2 {
			return match[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

func main() {
	configs := createConfigsModelFromEnvs()
	configs.print()
	if err := configs.validate(); err != nil {
		failf("Issue with input: %s", err)
	}

	log.SetEnableDebugLog(configs.VerboseLog == "yes")

	log.Infof("step determined configs:")

	// Detect Xcode major version
	xcodebuildVersion, err := utility.GetXcodeVersion()
	if err != nil {
		failf("Failed to get the version of xcodebuild! Error: %s", err)
	}
	log.Printf("- xcodebuild_version: %s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)

	// Detect xcpretty version
	if configs.OutputTool == "xcpretty" {
		xcprettyVersion, err := getXcprettyVersion()
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

	ideDistributionLogsZipPath := filepath.Join(configs.OutputDir, "xcodebuild.xcdistributionlogs.zip")
	log.Printf("- ideDistributionLogsZipPath: %s", ideDistributionLogsZipPath)

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

		log.TSuccessf("$ %s", xcprettyCmd.PrintableCmd())
		fmt.Println()

		if rawXcodebuildOut, err := xcprettyCmd.Run(); err != nil {

			log.Errorf("\nLast lines of the Xcode's build log:")
			fmt.Println(stringutil.LastNLines(rawXcodebuildOut, 10))

			if err := output.ExportOutputFileContent(rawXcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
				log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
			} else {
				log.Warnf(`You can find the last couple of lines of Xcode's build log above, but the full log is also available in the raw-xcodebuild-output.log
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path is available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable
(value: %s)`, rawXcodebuildOutputLogPath)
			}

			failf("Archive failed, error: %s", err)
		}
	} else {
		log.TSuccessf("$ %s", archiveCmd.PrintableCmd())
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

	archive, err := xcarchive.NewMacosArchive(archivePath)
	if err != nil {
		failf("Failed to parse archive, error: %s", err)
	}

	identity := archive.SigningIdentity()

	log.Infof("Archive infos:")
	log.Printf("codesign identity: %v", identity)
	fmt.Println()

	// Exporting xcarchive
	fmt.Println()
	log.Infof("Exporting xcarchive ...")
	fmt.Println()

	if err := output.ExportOutputDir(archivePath, archivePath, bitriseXCArchiveDirPthEnvKey); err != nil {
		failf("Failed to export %s, error: %s", bitriseXCArchiveDirPthEnvKey, err)
	}

	log.Donef("The xcarchive path is now available in the Environment Variable: %s (value: %s)", bitriseXCArchiveDirPthEnvKey, archivePath)

	if configs.IsExportXcarchiveZip == "yes" {
		if err := output.ZipAndExportOutput(archivePath, archiveZipPath, bitriseXCArchivePthEnvKey); err != nil {
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

		if err := output.ExportOutputDir(embeddedAppPath, appPath, bitriseAppPthEnvKey); err != nil {
			failf("Failed to export %s, error: %s", bitriseAppPthEnvKey, err)
		}

		log.Donef("The app path is now available in the Environment Variable: %s (value: %s)", bitriseAppPthEnvKey, appPath)

		filePath = filePath + ".zip"
		if err := output.ZipAndExportOutput(embeddedAppPath, filePath, bitriseExportedFilePath); err != nil {
			failf("Failed to export %s, error: %s", bitriseExportedFilePath, err)
		}

		log.Donef("The app.zip path is now available in the Environment Variable: %s (value: %s)", bitriseExportedFilePath, filePath)
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

		if configs.CustomExportOptionsPlistContent != "" {
			log.Printf("Custom export options content provided:")
			fmt.Println(configs.CustomExportOptionsPlistContent)

			if err := fileutil.WriteStringToFile(exportOptionsPath, configs.CustomExportOptionsPlistContent); err != nil {
				failf("Failed to write export options to file, error: %s", err)
			}
		} else {
			exportMethod, err := exportoptions.ParseMethod(configs.ExportMethod)
			if err != nil {
				failf("Failed to parse export method, error: %s", err)
			}

			bundleIDEntitlemnstMap := archive.BundleIDEntitlementsMap()
			bundleIDs := []string{}
			for bundleID := range bundleIDEntitlemnstMap {
				bundleIDs = append(bundleIDs, bundleID)
			}

			installedCertificates, err := certificateutil.InstalledCodesigningCertificateInfos()
			if err != nil {
				failf("Failed to get installed certificates, error: %s", err)
			}
			installedCertificates = certificateutil.FilterValidCertificateInfos(installedCertificates)

			log.Debugf("\n")
			log.Debugf("Installed certificates:")
			for _, certInfo := range installedCertificates {
				log.Debugf(certInfo.String())
			}

			installedProfiles, err := profileutil.InstalledProvisioningProfileInfos(profileutil.ProfileTypeMacOs)
			if err != nil {
				failf("Failed to get installed provisioning profiles, error: %s", err)
			}

			log.Debugf("\n")
			log.Debugf("Installed profiles:")
			for _, profInfo := range installedProfiles {
				log.Debugf(profInfo.String())
			}

			codesignGroups := export.CreateSelectableCodeSignGroups(installedCertificates, installedProfiles, bundleIDs)
			if len(codesignGroups) == 0 {
				log.Errorf("Failed to find code singing groups for the project")
			}

			codesignGroups = export.FilterSelectableCodeSignGroups(codesignGroups,
				export.CreateEntitlementsSelectableCodeSignGroupFilter(bundleIDEntitlemnstMap),
				export.CreateExportMethodSelectableCodeSignGroupFilter(exportMethod),
			)

			installedInstallerCertificates := []certificateutil.CertificateInfoModel{}

			if exportMethod == exportoptions.MethodAppStore {
				installedInstallerCertificates, err = certificateutil.InstalledInstallerCertificateInfos()
				if err != nil {
					log.Errorf("Failed to read installed Installer certificates, error: %s", err)
				}

				installedInstallerCertificates = certificateutil.FilterValidCertificateInfos(installedInstallerCertificates)

				log.Debugf("\n")
				log.Debugf("Installed installer certificates:")
				for _, certInfo := range installedInstallerCertificates {
					log.Debugf(certInfo.String())
				}
			}

			var macCodeSignGroup *export.MacCodeSignGroup
			macCodeSignGroups := export.CreateMacCodeSignGroup(codesignGroups, installedInstallerCertificates, exportMethod)
			if len(macCodeSignGroups) == 0 {
				log.Errorf("Can not create macos codesiging groups for the project")
			} else if len(macCodeSignGroups) > 1 {
				log.Warnf("Multiple matching  codesiging groups found for the project, using first...")
				macCodeSignGroup = &(macCodeSignGroups[0])
			} else {
				macCodeSignGroup = &(macCodeSignGroups[0])
			}

			exportProfileMapping := map[string]string{}
			if macCodeSignGroup != nil {
				for bundleID, profileInfo := range macCodeSignGroup.BundleIDProfileMap {
					exportProfileMapping[bundleID] = profileInfo.Name
				}
			}

			var exportOpts exportoptions.ExportOptions
			if exportMethod == exportoptions.MethodAppStore {
				options := exportoptions.NewAppStoreOptions()

				if macCodeSignGroup != nil {
					options.BundleIDProvisioningProfileMapping = exportProfileMapping
					options.SigningCertificate = macCodeSignGroup.Certificate.CommonName
					options.InstallerSigningCertificate = macCodeSignGroup.InstallerCertificate.CommonName
				}

				exportOpts = options
			} else {
				options := exportoptions.NewNonAppStoreOptions(exportMethod)

				if macCodeSignGroup != nil {
					options.BundleIDProvisioningProfileMapping = exportProfileMapping
					options.SigningCertificate = macCodeSignGroup.Certificate.CommonName
				}

				exportOpts = options
			}

			log.Printf("generated export options content:")
			fmt.Println()
			fmt.Println(exportOpts.String())

			if err = exportOpts.WriteToFile(exportOptionsPath); err != nil {
				failf("Failed to write export options to file, error: %s", err)
			}
		}

		exportCmd.SetExportOptionsPlist(exportOptionsPath)

		if configs.OutputTool == "xcpretty" {
			xcprettyCmd := xcpretty.New(exportCmd)

			log.Donef("$ %s", xcprettyCmd.PrintableCmd())
			fmt.Println()

			if xcodebuildOut, err := xcprettyCmd.Run(); err != nil {
				// xcodebuild raw output
				if err := output.ExportOutputFileContent(xcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
					log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
				} else {
					log.Warnf(`If you can't find the reason of the error in the log, please check the raw-xcodebuild-output.log
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable (value: %s)`, rawXcodebuildOutputLogPath)
				}

				// xcdistributionlogs
				if logsDirPth, err := findIDEDistrubutionLogsPath(xcodebuildOut); err != nil {
					log.Warnf("Failed to find xcdistributionlogs, error: %s", err)
				} else if err := output.ZipAndExportOutput(logsDirPth, ideDistributionLogsZipPath, bitriseIDEDistributionLogsPthEnvKey); err != nil {
					log.Warnf("Failed to export %s, error: %s", bitriseIDEDistributionLogsPthEnvKey, err)
				} else {
					criticalDistLogFilePth := filepath.Join(logsDirPth, "IDEDistribution.critical.log")
					log.Warnf("IDEDistribution.critical.log:")
					if criticalDistLog, err := fileutil.ReadStringFromFile(criticalDistLogFilePth); err == nil {
						log.Printf(criticalDistLog)
					}

					log.Warnf(`If you can't find the reason of the error in the log, please check the xcdistributionlogs
The logs directory is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_IDEDISTRIBUTION_LOGS_PATH environment variable (value: %s)`, ideDistributionLogsZipPath)
				}

				failf("Export failed, error: %s", err)
			}
		} else {
			log.Donef("$ %s", exportCmd.PrintableCmd())
			fmt.Println()

			if xcodebuildOut, err := exportCmd.RunAndReturnOutput(); err != nil {
				// xcdistributionlogs
				if logsDirPth, err := findIDEDistrubutionLogsPath(xcodebuildOut); err != nil {
					log.Warnf("Failed to find xcdistributionlogs, error: %s", err)
				} else if err := output.ZipAndExportOutput(logsDirPth, ideDistributionLogsZipPath, bitriseIDEDistributionLogsPthEnvKey); err != nil {
					log.Warnf("Failed to export %s, error: %s", bitriseIDEDistributionLogsPthEnvKey, err)
				} else {
					criticalDistLogFilePth := filepath.Join(logsDirPth, "IDEDistribution.critical.log")
					log.Warnf("IDEDistribution.critical.log:")
					if criticalDistLog, err := fileutil.ReadStringFromFile(criticalDistLogFilePth); err == nil {
						log.Printf(criticalDistLog)
					}

					log.Warnf(`If you can't find the reason of the error in the log, please check the xcdistributionlogs
The logs directory is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_IDEDISTRIBUTION_LOGS_PATH environment variable (value: %s)`, ideDistributionLogsZipPath)
				}

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
				if err := output.ExportOutputFile(apps[0], filePath, bitriseExportedFilePath); err != nil {
					failf("Failed to export %s, error: %s", bitriseExportedFilePath, err)
				}
			} else {
				if err := tools.ExportEnvironmentWithEnvman(bitriseAppPthEnvKey, filePath); err != nil {
					failf("Failed to export %s, error: %s", bitriseAppPthEnvKey, err)
				}
				filePath = filePath + ".zip"
				if err := output.ZipAndExportOutput(apps[0], filePath, bitriseExportedFilePath); err != nil {
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

	appDSYM, frameworkDSYMs, err := archive.FindDSYMs()
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

	if err := output.ZipAndExportOutput(dsymDir, dsymZipPath, bitriseDSYMDirPthEnvKey); err != nil {
		failf("Failed to export %s, error: %s", bitriseDSYMDirPthEnvKey, err)
	}

	log.Donef("The dSYM dir path is now available in the Environment Variable: %s (value: %s)", bitriseDSYMDirPthEnvKey, dsymZipPath)
}
