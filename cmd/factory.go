package cmd

import (
	"errors"
	"path/filepath"
	"time"

	boshlog "github.com/cloudfoundry/bosh-agent/logger"
	boshcmd "github.com/cloudfoundry/bosh-agent/platform/commands"
	boshsys "github.com/cloudfoundry/bosh-agent/system"
	boshtime "github.com/cloudfoundry/bosh-agent/time"
	boshuuid "github.com/cloudfoundry/bosh-agent/uuid"

	biblobstore "github.com/cloudfoundry/bosh-init/blobstore"
	bicloud "github.com/cloudfoundry/bosh-init/cloud"
	biconfig "github.com/cloudfoundry/bosh-init/config"
	bicrypto "github.com/cloudfoundry/bosh-init/crypto"
	bidepl "github.com/cloudfoundry/bosh-init/deployment"
	bihttpagent "github.com/cloudfoundry/bosh-init/deployment/agentclient/http"
	bidisk "github.com/cloudfoundry/bosh-init/deployment/disk"
	biinstance "github.com/cloudfoundry/bosh-init/deployment/instance"
	biinstancestate "github.com/cloudfoundry/bosh-init/deployment/instance/state"
	bideplmanifest "github.com/cloudfoundry/bosh-init/deployment/manifest"
	bideplrel "github.com/cloudfoundry/bosh-init/deployment/release"
	bisshtunnel "github.com/cloudfoundry/bosh-init/deployment/sshtunnel"
	bivm "github.com/cloudfoundry/bosh-init/deployment/vm"
	biindex "github.com/cloudfoundry/bosh-init/index"
	biinstall "github.com/cloudfoundry/bosh-init/installation"
	biinstallmanifest "github.com/cloudfoundry/bosh-init/installation/manifest"
	biregistry "github.com/cloudfoundry/bosh-init/registry"
	birel "github.com/cloudfoundry/bosh-init/release"
	birelset "github.com/cloudfoundry/bosh-init/release/set"
	birelsetmanifest "github.com/cloudfoundry/bosh-init/release/set/manifest"
	bistatepkg "github.com/cloudfoundry/bosh-init/state/pkg"
	bistemcell "github.com/cloudfoundry/bosh-init/stemcell"
	bitemplate "github.com/cloudfoundry/bosh-init/templatescompiler"
	bitemplateerb "github.com/cloudfoundry/bosh-init/templatescompiler/erbrenderer"
	biui "github.com/cloudfoundry/bosh-init/ui"
)

type Factory interface {
	CreateCommand(name string) (Cmd, error)
}

type factory struct {
	commands               map[string](func() (Cmd, error))
	fs                     boshsys.FileSystem
	ui                     biui.UI
	timeService            boshtime.Service
	logger                 boshlog.Logger
	uuidGenerator          boshuuid.Generator
	workspaceRootPath      string
	runner                 boshsys.CmdRunner
	compressor             boshcmd.Compressor
	agentClientFactory     bihttpagent.AgentClientFactory
	registryServerManager  biregistry.ServerManager
	sshTunnelFactory       bisshtunnel.Factory
	instanceFactory        biinstance.Factory
	instanceManagerFactory biinstance.ManagerFactory
	deploymentFactory      bidepl.Factory
	blobstoreFactory       biblobstore.Factory
	eventLogger            biui.Stage
	releaseExtractor       birel.Extractor
	releaseManager         birel.Manager
	releaseResolver        birelset.Resolver
	releaseSetParser       birelsetmanifest.Parser
	releaseJobResolver     bideplrel.JobResolver
	installationParser     biinstallmanifest.Parser
	deploymentParser       bideplmanifest.Parser
	releaseSetValidator    birelsetmanifest.Validator
	installationValidator  biinstallmanifest.Validator
	deploymentValidator    bideplmanifest.Validator
	cloudFactory           bicloud.Factory
	stateBuilderFactory    biinstancestate.BuilderFactory
	compiledPackageRepo    bistatepkg.CompiledPackageRepo
}

func NewFactory(
	fs boshsys.FileSystem,
	ui biui.UI,
	timeService boshtime.Service,
	logger boshlog.Logger,
	uuidGenerator boshuuid.Generator,
	workspaceRootPath string,
) Factory {
	f := &factory{
		fs:                fs,
		ui:                ui,
		timeService:       timeService,
		logger:            logger,
		uuidGenerator:     uuidGenerator,
		workspaceRootPath: workspaceRootPath,
	}
	f.commands = map[string](func() (Cmd, error)){
		"deploy": f.createDeployCmd,
		"delete": f.createDeleteCmd,
	}
	return f
}

func (f *factory) CreateCommand(name string) (Cmd, error) {
	if f.commands[name] == nil {
		return nil, errors.New("Invalid command name")
	}

	return f.commands[name]()
}

func (f *factory) createDeployCmd() (Cmd, error) {
	getter := func(deploymentManifestPath string) DeploymentPreparer {
		f := &deploymentManagerFactory2{f: f, deploymentManifestPath: deploymentManifestPath}
		return f.loadDeploymentPreparer()
	}
	return NewDeployCmd(f.ui, f.fs, f.logger, getter), nil
}

func (f *factory) createDeleteCmd() (Cmd, error) {
	getter := func(deploymentManifestPath string) DeploymentDeleter {
		f := &deploymentManagerFactory2{f: f, deploymentManifestPath: deploymentManifestPath}
		return f.loadDeploymentDeleter()
	}

	return NewDeleteCmd(f.ui, f.fs, f.logger, getter), nil
}

func (f *factory) loadCMDRunner() boshsys.CmdRunner {
	if f.runner != nil {
		return f.runner
	}
	f.runner = boshsys.NewExecCmdRunner(f.logger)
	return f.runner
}

func (f *factory) loadCompressor() boshcmd.Compressor {
	if f.compressor != nil {
		return f.compressor
	}
	f.compressor = boshcmd.NewTarballCompressor(f.loadCMDRunner(), f.fs)
	return f.compressor
}

func (f *factory) loadCompiledPackageRepo() bistatepkg.CompiledPackageRepo {
	if f.compiledPackageRepo != nil {
		return f.compiledPackageRepo
	}

	index := biindex.NewInMemoryIndex()
	f.compiledPackageRepo = bistatepkg.NewCompiledPackageRepo(index)
	return f.compiledPackageRepo
}

func (f *factory) loadRegistryServerManager() biregistry.ServerManager {
	if f.registryServerManager != nil {
		return f.registryServerManager
	}

	f.registryServerManager = biregistry.NewServerManager(f.logger)
	return f.registryServerManager
}

func (f *factory) loadSSHTunnelFactory() bisshtunnel.Factory {
	if f.sshTunnelFactory != nil {
		return f.sshTunnelFactory
	}

	f.sshTunnelFactory = bisshtunnel.NewFactory(f.logger)
	return f.sshTunnelFactory
}

func (f *factory) loadInstanceManagerFactory() biinstance.ManagerFactory {
	if f.instanceManagerFactory != nil {
		return f.instanceManagerFactory
	}

	f.instanceManagerFactory = biinstance.NewManagerFactory(
		f.loadSSHTunnelFactory(),
		f.loadInstanceFactory(),
		f.logger,
	)
	return f.instanceManagerFactory
}

func (f *factory) loadInstanceFactory() biinstance.Factory {
	if f.instanceFactory != nil {
		return f.instanceFactory
	}

	f.instanceFactory = biinstance.NewFactory(
		f.loadBuilderFactory(),
	)
	return f.instanceFactory
}

func (f *factory) loadReleaseJobResolver() bideplrel.JobResolver {
	if f.releaseJobResolver != nil {
		return f.releaseJobResolver
	}

	releaseSetResolver := birelset.NewResolver(f.loadReleaseManager(), f.logger)
	f.releaseJobResolver = bideplrel.NewJobResolver(releaseSetResolver)
	return f.releaseJobResolver
}

func (f *factory) loadBuilderFactory() biinstancestate.BuilderFactory {
	if f.stateBuilderFactory != nil {
		return f.stateBuilderFactory
	}

	erbRenderer := bitemplateerb.NewERBRenderer(f.fs, f.loadCMDRunner(), f.logger)
	jobRenderer := bitemplate.NewJobRenderer(erbRenderer, f.fs, f.logger)
	jobListRenderer := bitemplate.NewJobListRenderer(jobRenderer, f.logger)

	sha1Calculator := bicrypto.NewSha1Calculator(f.fs)

	renderedJobListCompressor := bitemplate.NewRenderedJobListCompressor(
		f.fs,
		f.loadCompressor(),
		sha1Calculator,
		f.logger,
	)

	f.stateBuilderFactory = biinstancestate.NewBuilderFactory(
		f.loadCompiledPackageRepo(),
		f.loadReleaseJobResolver(),
		jobListRenderer,
		renderedJobListCompressor,
		f.logger,
	)
	return f.stateBuilderFactory
}

func (f *factory) loadDeploymentFactory() bidepl.Factory {
	if f.deploymentFactory != nil {
		return f.deploymentFactory
	}

	pingTimeout := 10 * time.Second
	pingDelay := 500 * time.Millisecond

	f.deploymentFactory = bidepl.NewFactory(
		pingTimeout,
		pingDelay,
	)
	return f.deploymentFactory
}

func (f *factory) loadAgentClientFactory() bihttpagent.AgentClientFactory {
	if f.agentClientFactory != nil {
		return f.agentClientFactory
	}

	f.agentClientFactory = bihttpagent.NewAgentClientFactory(1*time.Second, f.logger)
	return f.agentClientFactory
}

func (f *factory) loadBlobstoreFactory() biblobstore.Factory {
	if f.blobstoreFactory != nil {
		return f.blobstoreFactory
	}

	f.blobstoreFactory = biblobstore.NewBlobstoreFactory(f.uuidGenerator, f.fs, f.logger)
	return f.blobstoreFactory
}

func (f *factory) loadReleaseExtractor() birel.Extractor {
	if f.releaseExtractor != nil {
		return f.releaseExtractor
	}

	releaseValidator := birel.NewValidator(f.fs)
	f.releaseExtractor = birel.NewExtractor(f.fs, f.loadCompressor(), releaseValidator, f.logger)
	return f.releaseExtractor
}

func (f *factory) loadReleaseManager() birel.Manager {
	if f.releaseManager != nil {
		return f.releaseManager
	}

	f.releaseManager = birel.NewManager(f.logger)
	return f.releaseManager
}

func (f *factory) loadReleaseResolver() birelset.Resolver {
	if f.releaseResolver != nil {
		return f.releaseResolver
	}

	f.releaseResolver = birelset.NewResolver(f.loadReleaseManager(), f.logger)
	return f.releaseResolver
}

func (f *factory) loadReleaseSetParser() birelsetmanifest.Parser {
	if f.releaseSetParser != nil {
		return f.releaseSetParser
	}

	f.releaseSetParser = birelsetmanifest.NewParser(f.fs, f.logger)
	return f.releaseSetParser
}

func (f *factory) loadInstallationParser() biinstallmanifest.Parser {
	if f.installationParser != nil {
		return f.installationParser
	}

	f.installationParser = biinstallmanifest.NewParser(f.fs, f.logger)
	return f.installationParser
}

func (f *factory) loadDeploymentParser() bideplmanifest.Parser {
	if f.deploymentParser != nil {
		return f.deploymentParser
	}

	f.deploymentParser = bideplmanifest.NewParser(f.fs, f.logger)
	return f.deploymentParser
}

func (f *factory) loadInstallationValidator() biinstallmanifest.Validator {
	if f.installationValidator != nil {
		return f.installationValidator
	}

	f.installationValidator = biinstallmanifest.NewValidator(f.logger, f.loadReleaseResolver())
	return f.installationValidator
}

func (f *factory) loadDeploymentValidator() bideplmanifest.Validator {
	if f.deploymentValidator != nil {
		return f.deploymentValidator
	}

	f.deploymentValidator = bideplmanifest.NewValidator(f.logger, f.loadReleaseResolver())
	return f.deploymentValidator
}

func (f *factory) loadReleaseSetValidator() birelsetmanifest.Validator {
	if f.releaseSetValidator != nil {
		return f.releaseSetValidator
	}

	f.releaseSetValidator = birelsetmanifest.NewValidator(f.logger, f.loadReleaseResolver())
	return f.releaseSetValidator
}

func (f *factory) loadCloudFactory() bicloud.Factory {
	if f.cloudFactory != nil {
		return f.cloudFactory
	}

	f.cloudFactory = bicloud.NewFactory(f.fs, f.loadCMDRunner(), f.logger)
	return f.cloudFactory
}

type deploymentManagerFactory2 struct {
	f                              *factory
	deploymentManifestPath         string
	deploymentConfigService        biconfig.DeploymentConfigService
	legacyDeploymentConfigMigrator biconfig.LegacyDeploymentConfigMigrator
	vmRepo                         biconfig.VMRepo
	stemcellRepo                   biconfig.StemcellRepo
	diskRepo                       biconfig.DiskRepo
	diskDeployer                   bivm.DiskDeployer
	diskManagerFactory             bidisk.ManagerFactory
	deploymentManagerFactory       bidepl.ManagerFactory
	vmManagerFactory               bivm.ManagerFactory
	stemcellManagerFactory         bistemcell.ManagerFactory
	installerFactory               biinstall.InstallerFactory
	deployer                       bidepl.Deployer
}

func (d *deploymentManagerFactory2) loadDeploymentPreparer() DeploymentPreparer {
	stemcellReader := bistemcell.NewReader(d.f.loadCompressor(), d.f.fs)
	stemcellExtractor := bistemcell.NewExtractor(stemcellReader, d.f.fs)
	deploymentRepo := biconfig.NewDeploymentRepo(d.loadDeploymentConfigService())
	releaseRepo := biconfig.NewReleaseRepo(d.loadDeploymentConfigService(), d.f.uuidGenerator)
	sha1Calculator := bicrypto.NewSha1Calculator(d.f.fs)
	deploymentRecord := bidepl.NewRecord(deploymentRepo, releaseRepo, d.loadStemcellRepo(), sha1Calculator)

	return NewDeploymentPreparer(
		d.f.ui,
		d.f.fs,
		d.f.logger,
		"DeploymentPreparer",
		d.loadDeploymentConfigService(),
		d.loadLegacyDeploymentConfigMigrator(),
		d.f.loadReleaseManager(),
		deploymentRecord,
		d.loadInstallerFactory(),
		d.f.loadCloudFactory(),
		d.loadStemcellManagerFactory(),
		d.f.loadAgentClientFactory(),
		d.loadVMManagerFactory(),
		d.f.loadBlobstoreFactory(),
		d.loadDeployer(),
		d.f.loadReleaseSetParser(),
		d.f.loadInstallationParser(),
		d.f.loadDeploymentParser(),
		d.f.loadReleaseSetValidator(),
		d.f.loadInstallationValidator(),
		d.f.loadDeploymentValidator(),
		d.f.loadReleaseExtractor(),
		d.f.loadReleaseResolver(),
		stemcellExtractor,
	)
}

func (d *deploymentManagerFactory2) loadDeploymentDeleter() DeploymentDeleter {
	return NewDeploymentDeleter(
		d.f.ui,
		"DeploymentDeleter",
		d.f.logger,
		d.f.fs,
		d.loadDeploymentConfigService(),
		d.f.loadReleaseManager(),
		d.loadInstallerFactory(),
		d.f.loadCloudFactory(),
		d.f.loadAgentClientFactory(),
		d.f.loadBlobstoreFactory(),
		d.loadDeploymentManagerFactory(),
		d.f.loadReleaseSetParser(),
		d.f.loadReleaseSetValidator(),
		d.f.loadReleaseResolver(),
		d.f.loadReleaseExtractor(),
		d.f.loadInstallationParser(),
		d.f.loadInstallationValidator(),
	)
}

func (d *deploymentManagerFactory2) loadDeploymentConfigService() biconfig.DeploymentConfigService {
	if d.deploymentConfigService != nil {
		return d.deploymentConfigService
	}

	d.deploymentConfigService = biconfig.NewFileSystemDeploymentConfigService(
		d.f.fs,
		d.f.uuidGenerator,
		d.f.logger,
		biconfig.DeploymentConfigPath(d.deploymentManifestPath),
	)
	return d.deploymentConfigService
}

func (d *deploymentManagerFactory2) loadLegacyDeploymentConfigMigrator() biconfig.LegacyDeploymentConfigMigrator {
	if d.legacyDeploymentConfigMigrator != nil {
		return d.legacyDeploymentConfigMigrator
	}

	d.legacyDeploymentConfigMigrator = biconfig.NewLegacyDeploymentConfigMigrator(
		d.loadDeploymentConfigService(),
		d.f.fs,
		d.f.uuidGenerator,
		d.f.logger,
	)
	return d.legacyDeploymentConfigMigrator
}

func (d *deploymentManagerFactory2) loadStemcellRepo() biconfig.StemcellRepo {
	if d.stemcellRepo != nil {
		return d.stemcellRepo
	}
	d.stemcellRepo = biconfig.NewStemcellRepo(d.loadDeploymentConfigService(), d.f.uuidGenerator)
	return d.stemcellRepo
}

func (d *deploymentManagerFactory2) loadVMRepo() biconfig.VMRepo {
	if d.vmRepo != nil {
		return d.vmRepo
	}
	d.vmRepo = biconfig.NewVMRepo(d.loadDeploymentConfigService())
	return d.vmRepo
}

func (d *deploymentManagerFactory2) loadDiskRepo() biconfig.DiskRepo {
	if d.diskRepo != nil {
		return d.diskRepo
	}
	d.diskRepo = biconfig.NewDiskRepo(d.loadDeploymentConfigService(), d.f.uuidGenerator)
	return d.diskRepo
}

func (d *deploymentManagerFactory2) loadDiskDeployer() bivm.DiskDeployer {
	if d.diskDeployer != nil {
		return d.diskDeployer
	}

	d.diskDeployer = bivm.NewDiskDeployer(d.loadDiskManagerFactory(), d.loadDiskRepo(), d.f.logger)
	return d.diskDeployer
}

func (d *deploymentManagerFactory2) loadDiskManagerFactory() bidisk.ManagerFactory {
	if d.diskManagerFactory != nil {
		return d.diskManagerFactory
	}

	d.diskManagerFactory = bidisk.NewManagerFactory(d.loadDiskRepo(), d.f.logger)
	return d.diskManagerFactory
}

func (d *deploymentManagerFactory2) loadDeploymentManagerFactory() bidepl.ManagerFactory {
	if d.deploymentManagerFactory != nil {
		return d.deploymentManagerFactory
	}

	d.deploymentManagerFactory = bidepl.NewManagerFactory(
		d.loadVMManagerFactory(),
		d.f.loadInstanceManagerFactory(),
		d.loadDiskManagerFactory(),
		d.loadStemcellManagerFactory(),
		d.f.loadDeploymentFactory(),
	)
	return d.deploymentManagerFactory
}

func (d *deploymentManagerFactory2) loadVMManagerFactory() bivm.ManagerFactory {
	if d.vmManagerFactory != nil {
		return d.vmManagerFactory
	}

	d.vmManagerFactory = bivm.NewManagerFactory(
		d.loadVMRepo(),
		d.loadStemcellRepo(),
		d.loadDiskDeployer(),
		d.f.uuidGenerator,
		d.f.fs,
		d.f.logger,
	)
	return d.vmManagerFactory
}

func (d *deploymentManagerFactory2) loadStemcellManagerFactory() bistemcell.ManagerFactory {
	if d.stemcellManagerFactory != nil {
		return d.stemcellManagerFactory
	}

	d.stemcellManagerFactory = bistemcell.NewManagerFactory(d.loadStemcellRepo())
	return d.stemcellManagerFactory
}

func (d *deploymentManagerFactory2) loadDeployer() bidepl.Deployer {
	if d.deployer != nil {
		return d.deployer
	}

	d.deployer = bidepl.NewDeployer(
		d.loadVMManagerFactory(),
		d.f.loadInstanceManagerFactory(),
		d.f.loadDeploymentFactory(),
		d.f.logger,
	)
	return d.deployer
}

func (d *deploymentManagerFactory2) loadInstallerFactory() biinstall.InstallerFactory {
	if d.installerFactory != nil {
		return d.installerFactory
	}

	targetProvider := biinstall.NewTargetProvider(
		d.loadDeploymentConfigService(),
		d.f.uuidGenerator,
		filepath.Join(d.f.workspaceRootPath, "installations"),
	)

	d.installerFactory = biinstall.NewInstallerFactory(
		targetProvider,
		d.f.ui,
		d.f.fs,
		d.f.loadCMDRunner(),
		d.f.loadCompressor(),
		d.f.loadReleaseResolver(),
		d.f.loadReleaseJobResolver(),
		d.f.uuidGenerator,
		d.f.loadRegistryServerManager(),
		d.f.logger,
	)
	return d.installerFactory
}
