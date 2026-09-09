// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

package cmdrunner

// addrTranslator implements stateless identity functions, as the host and core
// run in the same context wrt Unix and network addresses.
type addrTranslator struct{}

func (*addrTranslator) PluginToHost(coreNet, coreAddr string) (string, string, error) {
	return coreNet, coreAddr, nil
}

func (*addrTranslator) HostToPlugin(hostNet, hostAddr string) (string, string, error) {
	return hostNet, hostAddr, nil
}
