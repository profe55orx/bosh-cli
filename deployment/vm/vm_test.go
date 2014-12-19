package vm_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	fakesys "github.com/cloudfoundry/bosh-agent/system/fakes"
	fakebmcloud "github.com/cloudfoundry/bosh-micro-cli/cloud/fakes"
	fakebmconfig "github.com/cloudfoundry/bosh-micro-cli/config/fakes"
	fakebmagentclient "github.com/cloudfoundry/bosh-micro-cli/deployment/agentclient/fakes"
	fakebmas "github.com/cloudfoundry/bosh-micro-cli/deployment/applyspec/fakes"
	fakebmdisk "github.com/cloudfoundry/bosh-micro-cli/deployment/disk/fakes"
	fakebmvm "github.com/cloudfoundry/bosh-micro-cli/deployment/vm/fakes"
	fakebmlog "github.com/cloudfoundry/bosh-micro-cli/eventlogger/fakes"

	boshlog "github.com/cloudfoundry/bosh-agent/logger"
	bmcloud "github.com/cloudfoundry/bosh-micro-cli/cloud"
	bmconfig "github.com/cloudfoundry/bosh-micro-cli/config"
	bmagentclient "github.com/cloudfoundry/bosh-micro-cli/deployment/agentclient"
	bmas "github.com/cloudfoundry/bosh-micro-cli/deployment/applyspec"
	bmdisk "github.com/cloudfoundry/bosh-micro-cli/deployment/disk"
	bmmanifest "github.com/cloudfoundry/bosh-micro-cli/deployment/manifest"
	bmstemcell "github.com/cloudfoundry/bosh-micro-cli/deployment/stemcell"

	. "github.com/cloudfoundry/bosh-micro-cli/deployment/vm"
)

var _ = Describe("VM", func() {
	var (
		vm                         VM
		fakeVMRepo                 *fakebmconfig.FakeVMRepo
		fakeStemcellRepo           *fakebmconfig.FakeStemcellRepo
		fakeDiskDeployer           *fakebmvm.FakeDiskDeployer
		fakeAgentClient            *fakebmagentclient.FakeAgentClient
		fakeCloud                  *fakebmcloud.FakeCloud
		applySpec                  bmstemcell.ApplySpec
		fakeTemplatesSpecGenerator *fakebmas.FakeTemplatesSpecGenerator
		fakeApplySpecFactory       *fakebmas.FakeApplySpecFactory
		deploymentManifest         bmmanifest.Manifest
		diskPool                   bmmanifest.DiskPool
		deploymentJob              bmmanifest.Job
		stemcellJob                bmstemcell.Job
		fs                         *fakesys.FakeFileSystem
		logger                     boshlog.Logger
	)

	BeforeEach(func() {
		fakeTemplatesSpecGenerator = fakebmas.NewFakeTemplatesSpecGenerator()
		fakeTemplatesSpecGenerator.SetCreateBehavior(bmas.TemplatesSpec{
			BlobID:            "fake-blob-id",
			ArchiveSha1:       "fake-archive-sha1",
			ConfigurationHash: "fake-configuration-hash",
		}, nil)

		fakeAgentClient = fakebmagentclient.NewFakeAgentClient()

		diskPool = bmmanifest.DiskPool{
			Name:     "fake-persistent-disk-pool-name",
			DiskSize: 1024,
			RawCloudProperties: map[interface{}]interface{}{
				"fake-disk-pool-cloud-property-key": "fake-disk-pool-cloud-property-value",
			},
		}

		stemcellJob = bmstemcell.Job{
			Name: "fake-job-name",
			Templates: []bmstemcell.Blob{
				{
					Name:        "first-job-name",
					Version:     "first-job-version",
					SHA1:        "first-job-sha1",
					BlobstoreID: "first-job-blobstore-id",
				},
				{
					Name:        "second-job-name",
					Version:     "second-job-version",
					SHA1:        "second-job-sha1",
					BlobstoreID: "second-job-blobstore-id",
				},
				{
					Name:        "third-job-name",
					Version:     "third-job-version",
					SHA1:        "third-job-sha1",
					BlobstoreID: "third-job-blobstore-id",
				},
			},
		}
		applySpec = bmstemcell.ApplySpec{
			Packages: map[string]bmstemcell.Blob{
				"first-package-name": bmstemcell.Blob{
					Name:        "first-package-name",
					Version:     "first-package-version",
					SHA1:        "first-package-sha1",
					BlobstoreID: "first-package-blobstore-id",
				},
				"second-package-name": bmstemcell.Blob{
					Name:        "second-package-name",
					Version:     "second-package-version",
					SHA1:        "second-package-sha1",
					BlobstoreID: "second-package-blobstore-id",
				},
			},
			Job: stemcellJob,
		}

		deploymentJob = bmmanifest.Job{
			Name: "fake-manifest-job-name",
			Templates: []bmmanifest.ReleaseJobRef{
				{Name: "first-job-name"},
				{Name: "third-job-name"},
			},
			PersistentDiskPool: "fake-persistent-disk-pool-name",
			RawProperties: map[interface{}]interface{}{
				"fake-property-key": "fake-property-value",
			},
			Networks: []bmmanifest.JobNetwork{
				{
					Name:      "fake-network-name",
					StaticIPs: []string{"fake-network-ip"},
				},
			},
		}
		deploymentManifest = bmmanifest.Manifest{
			Name: "fake-deployment-name",
			DiskPools: []bmmanifest.DiskPool{
				diskPool,
			},
			Jobs: []bmmanifest.Job{
				deploymentJob,
			},
			Networks: []bmmanifest.Network{
				{
					Name: "fake-network-name",
					Type: "fake-network-type",
				},
			},
		}

		fakeApplySpecFactory = fakebmas.NewFakeApplySpecFactory()

		logger = boshlog.NewLogger(boshlog.LevelNone)
		fs = fakesys.NewFakeFileSystem()
		fakeCloud = fakebmcloud.NewFakeCloud()
		fakeVMRepo = fakebmconfig.NewFakeVMRepo()
		fakeStemcellRepo = fakebmconfig.NewFakeStemcellRepo()
		fakeDiskDeployer = fakebmvm.NewFakeDiskDeployer()
		vm = NewVM(
			"fake-vm-cid",
			fakeVMRepo,
			fakeStemcellRepo,
			fakeDiskDeployer,
			fakeAgentClient,
			fakeCloud,
			fakeTemplatesSpecGenerator,
			fakeApplySpecFactory,
			"fake-mbus-url",
			fs,
			logger,
		)
	})

	Describe("Exists", func() {
		It("returns true when the vm exists", func() {
			fakeCloud.HasVMFound = true

			exists, err := vm.Exists()
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("returns false when the vm does not exist", func() {
			fakeCloud.HasVMFound = false

			exists, err := vm.Exists()
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("returns error when checking fails", func() {
			fakeCloud.HasVMErr = errors.New("fake-has-vm-error")

			_, err := vm.Exists()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fake-has-vm-error"))
		})
	})

	Describe("UpdateDisks", func() {
		It("delegates to DiskDeployer.Deploy", func() {
			fakeStage := fakebmlog.NewFakeStage()

			err := vm.UpdateDisks(diskPool, fakeStage)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeDiskDeployer.DeployInputs).To(Equal([]fakebmvm.DeployInput{
				{
					DiskPool:         diskPool,
					Cloud:            fakeCloud,
					VM:               vm,
					EventLoggerStage: fakeStage,
				},
			}))
		})
	})

	Describe("Apply", func() {
		It("stops the agent", func() {
			err := vm.Apply(applySpec, deploymentManifest)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeAgentClient.StopCalled).To(BeTrue())
		})

		It("generates templates spec", func() {
			err := vm.Apply(applySpec, deploymentManifest)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeTemplatesSpecGenerator.CreateTemplatesSpecInputs).To(ContainElement(fakebmas.CreateTemplatesSpecInput{
				DeploymentJob:  deploymentJob,
				StemcellJob:    stemcellJob,
				DeploymentName: "fake-deployment-name",
				Properties: map[string]interface{}{
					"fake-property-key": "fake-property-value",
				},
				MbusURL: "fake-mbus-url",
			}))
		})

		It("creates apply spec", func() {
			err := vm.Apply(applySpec, deploymentManifest)
			Expect(err).ToNot(HaveOccurred())

			Expect(fakeApplySpecFactory.CreateInput).To(Equal(
				fakebmas.CreateInput{
					ApplySpec:      applySpec,
					DeploymentName: "fake-deployment-name",
					JobName:        "fake-manifest-job-name",
					NetworksSpec: map[string]interface{}{
						"fake-network-name": map[string]interface{}{
							"type":             "fake-network-type",
							"ip":               "fake-network-ip",
							"cloud_properties": map[string]interface{}{},
						},
					},
					ArchivedTemplatesBlobID: "fake-blob-id",
					ArchivedTemplatesSha1:   "fake-archive-sha1",
					TemplatesDirSha1:        "fake-configuration-hash",
				},
			))
		})

		It("sends apply spec to the agent", func() {
			agentApplySpec := bmas.ApplySpec{
				Deployment: "fake-deployment-name",
			}
			fakeApplySpecFactory.CreateApplySpec = agentApplySpec
			err := vm.Apply(applySpec, deploymentManifest)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeAgentClient.ApplyApplySpec).To(Equal(agentApplySpec))
		})

		Context("when creating templates spec fails", func() {
			BeforeEach(func() {
				fakeTemplatesSpecGenerator.CreateErr = errors.New("fake-template-err")
			})

			It("returns an error", func() {
				err := vm.Apply(applySpec, deploymentManifest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-template-err"))
			})
		})

		Context("when sending apply spec to the agent fails", func() {
			BeforeEach(func() {
				fakeAgentClient.ApplyErr = errors.New("fake-agent-apply-err")
			})

			It("returns an error", func() {
				err := vm.Apply(applySpec, deploymentManifest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-agent-apply-err"))
			})
		})

		Context("when stopping an agent fails", func() {
			BeforeEach(func() {
				fakeAgentClient.SetStopBehavior(errors.New("fake-stop-error"))
			})

			It("returns an error", func() {
				err := vm.Apply(applySpec, deploymentManifest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-stop-error"))
			})
		})
	})

	Describe("Start", func() {
		It("starts agent services", func() {
			err := vm.Start()
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeAgentClient.StartCalled).To(BeTrue())
		})

		Context("when starting an agent fails", func() {
			BeforeEach(func() {
				fakeAgentClient.SetStartBehavior(errors.New("fake-start-error"))
			})

			It("returns an error", func() {
				err := vm.Start()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-start-error"))
			})
		})
	})

	Describe("WaitToBeRunning", func() {
		BeforeEach(func() {
			fakeAgentClient.SetGetStateBehavior(bmagentclient.State{JobState: "pending"}, nil)
			fakeAgentClient.SetGetStateBehavior(bmagentclient.State{JobState: "pending"}, nil)
			fakeAgentClient.SetGetStateBehavior(bmagentclient.State{JobState: "running"}, nil)
		})

		It("waits until agent reports state as running", func() {
			err := vm.WaitToBeRunning(5, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeAgentClient.GetStateCalledTimes).To(Equal(3))
		})
	})

	Describe("AttachDisk", func() {
		var disk *fakebmdisk.FakeDisk

		BeforeEach(func() {
			disk = fakebmdisk.NewFakeDisk("fake-disk-cid")
		})

		It("attaches disk to vm in the cloud", func() {
			err := vm.AttachDisk(disk)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeCloud.AttachDiskInput).To(Equal(fakebmcloud.AttachDiskInput{
				VMCID:   "fake-vm-cid",
				DiskCID: "fake-disk-cid",
			}))
		})

		It("sends mount disk to the agent", func() {
			err := vm.AttachDisk(disk)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeAgentClient.MountDiskCID).To(Equal("fake-disk-cid"))
		})

		Context("when attaching disk to cloud fails", func() {
			BeforeEach(func() {
				fakeCloud.AttachDiskErr = errors.New("fake-attach-error")
			})

			It("returns an error", func() {
				err := vm.AttachDisk(disk)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-attach-error"))
			})
		})

		Context("when mounting disk fails", func() {
			BeforeEach(func() {
				fakeAgentClient.SetMountDiskBehavior(errors.New("fake-mount-error"))
			})

			It("returns an error", func() {
				err := vm.AttachDisk(disk)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-mount-error"))
			})
		})
	})

	Describe("DetachDisk", func() {
		var disk *fakebmdisk.FakeDisk

		BeforeEach(func() {
			disk = fakebmdisk.NewFakeDisk("fake-disk-cid")
		})

		It("detaches disk from vm in the cloud", func() {
			err := vm.DetachDisk(disk)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeCloud.DetachDiskInput).To(Equal(fakebmcloud.DetachDiskInput{
				VMCID:   "fake-vm-cid",
				DiskCID: "fake-disk-cid",
			}))
		})

		Context("when detaching disk to cloud fails", func() {
			BeforeEach(func() {
				fakeCloud.DetachDiskErr = errors.New("fake-detach-error")
			})

			It("returns an error", func() {
				err := vm.DetachDisk(disk)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-detach-error"))
			})
		})
	})

	Describe("UnmountDisk", func() {
		var disk *fakebmdisk.FakeDisk

		BeforeEach(func() {
			disk = fakebmdisk.NewFakeDisk("fake-disk-cid")
		})

		It("sends unmount disk to the agent", func() {
			err := vm.UnmountDisk(disk)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeAgentClient.UnmountDiskCID).To(Equal("fake-disk-cid"))
		})

		Context("when unmounting disk fails", func() {
			BeforeEach(func() {
				fakeAgentClient.SetUnmountDiskBehavior(errors.New("fake-unmount-error"))
			})

			It("returns an error", func() {
				err := vm.UnmountDisk(disk)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-unmount-error"))
			})
		})
	})

	Describe("Stop", func() {
		It("stops agent services", func() {
			err := vm.Stop()
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeAgentClient.StopCalled).To(BeTrue())
		})

		Context("when stopping an agent fails", func() {
			BeforeEach(func() {
				fakeAgentClient.SetStopBehavior(errors.New("fake-stop-error"))
			})

			It("returns an error", func() {
				err := vm.Stop()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-stop-error"))
			})
		})
	})

	Describe("Disks", func() {
		BeforeEach(func() {
			fakeAgentClient.SetListDiskBehavior([]string{"fake-disk-cid-1", "fake-disk-cid-2"}, nil)
		})

		It("returns disks that are reported by the agent", func() {
			disks, err := vm.Disks()
			Expect(err).ToNot(HaveOccurred())
			expectedFirstDisk := bmdisk.NewDisk(bmconfig.DiskRecord{CID: "fake-disk-cid-1"}, nil, nil)
			expectedSecondDisk := bmdisk.NewDisk(bmconfig.DiskRecord{CID: "fake-disk-cid-2"}, nil, nil)
			Expect(disks).To(Equal([]bmdisk.Disk{expectedFirstDisk, expectedSecondDisk}))
		})

		Context("when listing disks fails", func() {
			BeforeEach(func() {
				fakeAgentClient.SetListDiskBehavior([]string{}, errors.New("fake-list-disk-error"))
			})

			It("returns an error", func() {
				_, err := vm.Disks()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-list-disk-error"))
			})
		})
	})

	Describe("Delete", func() {
		It("deletes vm in the cloud", func() {
			err := vm.Delete()
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeCloud.DeleteVMInput).To(Equal(fakebmcloud.DeleteVMInput{
				VMCID: "fake-vm-cid",
			}))
		})

		It("deletes VM in the vm repo", func() {
			err := vm.Delete()
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeVMRepo.ClearCurrentCalled).To(BeTrue())
		})

		It("clears current stemcell in the stemcell repo", func() {
			err := vm.Delete()
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeStemcellRepo.ClearCurrentCalled).To(BeTrue())
		})

		Context("when deleting vm in the cloud fails", func() {
			BeforeEach(func() {
				fakeCloud.DeleteVMErr = errors.New("fake-delete-vm-error")
			})

			It("returns an error", func() {
				err := vm.Delete()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-delete-vm-error"))
			})
		})

		Context("when deleting vm in the cloud fails with VMNotFoundError", func() {
			var deleteErr = bmcloud.NewCPIError("delete_vm", bmcloud.CmdError{
				Type:    bmcloud.VMNotFoundError,
				Message: "fake-vm-not-found-message",
			})

			BeforeEach(func() {
				fakeCloud.DeleteVMErr = deleteErr
			})

			It("deletes vm in the cloud", func() {
				err := vm.Delete()
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(deleteErr))
				Expect(fakeCloud.DeleteVMInput).To(Equal(fakebmcloud.DeleteVMInput{
					VMCID: "fake-vm-cid",
				}))
			})

			It("deletes VM in the vm repo", func() {
				err := vm.Delete()
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(deleteErr))
				Expect(fakeVMRepo.ClearCurrentCalled).To(BeTrue())
			})

			It("clears current stemcell in the stemcell repo", func() {
				err := vm.Delete()
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(deleteErr))
				Expect(fakeStemcellRepo.ClearCurrentCalled).To(BeTrue())
			})
		})
	})

	Describe("MigrateDisk", func() {
		It("sends migrate_disk to the agent", func() {
			err := vm.MigrateDisk()
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeAgentClient.MigrateDiskCalledTimes).To(Equal(1))
		})

		Context("when migrating disk fails", func() {
			BeforeEach(func() {
				fakeAgentClient.SetMigrateDiskBehavior(errors.New("fake-migrate-error"))
			})

			It("returns an error", func() {
				err := vm.MigrateDisk()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-migrate-error"))
			})
		})
	})
})