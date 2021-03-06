// +build linux darwin

package smbdriver_test

import (
	"context"
	"fmt"
	"os"
	"strings"

	"code.cloudfoundry.org/dockerdriver"
	"code.cloudfoundry.org/dockerdriver/dockerdriverfakes"
	"code.cloudfoundry.org/dockerdriver/driverhttp"
	"code.cloudfoundry.org/goshims/ioutilshim/ioutil_fake"
	"code.cloudfoundry.org/goshims/osshim/os_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/smbdriver"
	"code.cloudfoundry.org/volumedriver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SmbMounter", func() {
	var (
		logger      lager.Logger
		testContext context.Context
		env         dockerdriver.Env
		err         error

		fakeInvoker *dockerdriverfakes.FakeInvoker
		fakeIoutil  *ioutil_fake.FakeIoutil
		fakeOs      *os_fake.FakeOs

		subject volumedriver.Mounter

		opts map[string]interface{}
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("smb-mounter")
		testContext = context.TODO()
		env = driverhttp.NewHttpDriverEnv(logger, testContext)
		opts = map[string]interface{}{}

		fakeInvoker = &dockerdriverfakes.FakeInvoker{}
		fakeIoutil = &ioutil_fake.FakeIoutil{}
		fakeOs = &os_fake.FakeOs{}

		config := smbdriver.NewSmbConfig()
		_ = config.ReadConf("username,password,vers,uid,gid,file_mode,dir_mode,readonly,ro", "", []string{})

		subject = smbdriver.NewSmbMounter(fakeInvoker, fakeOs, fakeIoutil, config)
	})

	Context("#Mount", func() {
		Context("when mount succeeds", func() {
			JustBeforeEach(func() {
				fakeInvoker.InvokeReturns(nil, nil)
				err = subject.Mount(env, "source", "target", opts)
			})

			It("should return without error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should use the passed in variables", func() {
				_, cmd, args := fakeInvoker.InvokeArgsForCall(0)
				Expect(cmd).To(Equal("mount"))
				Expect(strings.Join(args, " ")).To(ContainSubstring("source"))
				Expect(strings.Join(args, " ")).To(ContainSubstring("target"))
			})

			Context("when mounting read only with readonly", func() {
				Context("and readonly is passed", func() {
					BeforeEach(func() {
						opts["readonly"] = true
					})

					It("should include the ro flag", func() {
						_, _, args := fakeInvoker.InvokeArgsForCall(0)
						Expect(strings.Join(args, " ")).To(ContainSubstring("ro"))
					})
				})

				Context("and ro is passed", func() {
					BeforeEach(func() {
						opts["ro"] = true
					})

					It("should include the ro flag", func() {
						_, _, args := fakeInvoker.InvokeArgsForCall(0)
						Expect(strings.Join(args, " ")).To(ContainSubstring("ro"))
					})
				})
			})
		})

		Context("when mount errors", func() {
			BeforeEach(func() {
				fakeInvoker.InvokeReturns([]byte("error"), fmt.Errorf("error"))

				err = subject.Mount(env, "source", "target", opts)
			})

			It("should return with error", func() {
				Expect(err).To(HaveOccurred())
				_, ok := err.(dockerdriver.SafeError)
				Expect(ok).To(BeTrue())
			})
		})

		Context("when mount is cancelled", func() {
			// TODO: when we pick up the lager.Context
		})

		Context("when error occurs", func() {
			BeforeEach(func() {
				opts = map[string]interface{}{}

				config := smbdriver.NewSmbConfig()
				_ = config.ReadConf("password,vers,file_mode,dir_mode,readonly", "", []string{"username"})

				subject = smbdriver.NewSmbMounter(fakeInvoker, fakeOs, fakeIoutil, config)

				fakeInvoker.InvokeReturns(nil, nil)
			})

			JustBeforeEach(func() {
				err = subject.Mount(env, "source", "target", opts)
			})

			Context("when a required option is missing", func() {
				It("should error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Missing mandatory options"))
					_, ok := err.(dockerdriver.SafeError)
					Expect(ok).To(BeTrue())
				})
			})

			Context("when a disallowed option is passed", func() {
				BeforeEach(func() {
					opts["username"] = "fake-username"
					opts["uid"] = "uid"
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Not allowed options"))
					_, ok := err.(dockerdriver.SafeError)
					Expect(ok).To(BeTrue())
				})
			})
		})
	})

	Context("#Unmount", func() {
		Context("when mount succeeds", func() {
			BeforeEach(func() {
				fakeInvoker.InvokeReturns(nil, nil)

				err = subject.Unmount(env, "target")
			})

			It("should return without error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should use the passed in variables", func() {
				_, cmd, args := fakeInvoker.InvokeArgsForCall(0)
				Expect(cmd).To(Equal("umount"))
				Expect(len(args)).To(Equal(2))
				Expect(args[0]).To(Equal("-l"))
				Expect(args[1]).To(Equal("target"))
			})
		})

		Context("when unmount fails", func() {
			BeforeEach(func() {
				fakeInvoker.InvokeReturns([]byte("error"), fmt.Errorf("error"))
				err = subject.Unmount(env, "target")
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())

				_, ok := err.(dockerdriver.SafeError)
				Expect(ok).To(BeTrue())
			})
		})
	})

	Context("#Check", func() {
		var (
			success bool
		)

		Context("when check succeeds", func() {
			BeforeEach(func() {
				success = subject.Check(env, "target", "source")
			})
			It("uses correct context", func() {
				env, _, _ := fakeInvoker.InvokeArgsForCall(0)
				Expect(fmt.Sprintf("%#v", env.Context())).To(ContainSubstring("timerCtx"))
			})
			It("reports valid mountpoint", func() {
				Expect(success).To(BeTrue())
			})
		})

		Context("when check fails", func() {
			BeforeEach(func() {
				fakeInvoker.InvokeReturns([]byte("error"), fmt.Errorf("error"))
				success = subject.Check(env, "target", "source")
			})
			It("reports invalid mountpoint", func() {
				Expect(success).To(BeFalse())
			})
		})
	})

	Context("#Purge", func() {
		JustBeforeEach(func() {
			subject.Purge(env, "/var/vcap/data/some/path")
		})

		Context("when stuff is in the directory", func() {
			var fakeStuff *ioutil_fake.FakeFileInfo

			BeforeEach(func() {
				fakeStuff = &ioutil_fake.FakeFileInfo{}
				fakeStuff.NameReturns("guidy-guid-guid")
				fakeStuff.IsDirReturns(true)

				fakeIoutil.ReadDirReturns([]os.FileInfo{fakeStuff}, nil)
			})

			It("should attempt to unmount the directory", func() {
				Expect(fakeInvoker.InvokeCallCount()).To(Equal(1))

				_, proc, args := fakeInvoker.InvokeArgsForCall(0)
				Expect(proc).To(Equal("umount"))
				Expect(len(args)).To(Equal(3))
				Expect(args[0]).To(Equal("-l"))
				Expect(args[1]).To(Equal("-f"))
				Expect(args[2]).To(Equal("/var/vcap/data/some/path/guidy-guid-guid"))
			})

			It("should remove the mount directory", func() {
				Expect(fakeOs.RemoveCallCount()).To(Equal(1))

				path := fakeOs.RemoveArgsForCall(0)
				Expect(path).To(Equal("/var/vcap/data/some/path/guidy-guid-guid"))
			})

			Context("when the stuff is not a directory", func() {
				BeforeEach(func() {
					fakeStuff.IsDirReturns(false)
				})

				It("should not remove the stuff", func() {
					Expect(fakeOs.RemoveCallCount()).To(BeZero())
				})
			})
		})
	})
})
