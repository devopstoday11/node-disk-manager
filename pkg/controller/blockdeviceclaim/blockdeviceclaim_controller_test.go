package blockdeviceclaim

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"
	"time"

	ndm "github.com/openebs/node-disk-manager/cmd/ndm_daemonset/controller"
	openebsv1alpha1 "github.com/openebs/node-disk-manager/pkg/apis/openebs/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	fakeHostName                   = "fake-hostname"
	diskName                       = "disk-example"
	deviceName                     = "blockdevice-example"
	blockDeviceClaimName           = "blockdeviceclaim-example"
	blockDeviceClaimUID  types.UID = "blockDeviceClaim-example-UID"
	namespace                      = ""
	capacity             uint64    = 1024000
	claimCapacity                  = resource.MustParse("1024000")
)

// TestBlockDeviceClaimController runs ReconcileBlockDeviceClaim.Reconcile() against a
// fake client that tracks a BlockDeviceClaim object.
// Test description:
func TestBlockDeviceClaimController(t *testing.T) {

	// Set the logger to development mode for verbose logs.
	logf.SetLogger(logf.ZapLogger(true))

	// Create a fake client to mock API calls.
	cl, s := CreateFakeClient(t)

	// Create a ReconcileDevice object with the scheme and fake client.
	r := &ReconcileBlockDeviceClaim{client: cl, scheme: s}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      blockDeviceClaimName,
			Namespace: namespace,
		},
	}

	// Check status of deviceClaim it should be empty(Not bound)
	r.CheckBlockDeviceClaimStatus(t, req, openebsv1alpha1.BlockDeviceClaimStatusEmpty)

	// Fetch the BlockDeviceClaim CR and change capacity to invalid
	// Since Capacity is invalid, it delete device claim CR
	r.InvalidCapacityTest(t, req)

	// Create new BlockDeviceClaim CR with right capacity,
	// trigger reconilation event. This time, it should
	// bound.
	blockDeviceClaimCR := GetFakeBlockDeviceClaimObject()
	err := r.client.Create(context.TODO(), blockDeviceClaimCR)
	if err != nil {
		t.Errorf("BlockDeviceClaim object is not created")
	}

	res, err := r.Reconcile(req)
	if err != nil {
		t.Logf("reconcile: (%v)", err)
	}

	// Check the result of reconciliation to make sure it has the desired state.
	if !res.Requeue {
		t.Log("reconcile did not requeue request as expected")
	}
	r.CheckBlockDeviceClaimStatus(t, req, openebsv1alpha1.BlockDeviceClaimStatusDone)

	r.DeviceRequestedHappyPathTest(t, req)
	//TODO: Need to find a way to update deletion timestamp
	//r.DeleteBlockDeviceClaimedTest(t, req)
}

func (r *ReconcileBlockDeviceClaim) DeleteBlockDeviceClaimedTest(t *testing.T,
	req reconcile.Request) {

	devRequestInst := &openebsv1alpha1.BlockDeviceClaim{}

	// Fetch the BlockDeviceClaim CR
	err := r.client.Get(context.TODO(), req.NamespacedName, devRequestInst)
	if err != nil {
		t.Errorf("Get devClaimInst: (%v)", err)
	}

	err = r.client.Delete(context.TODO(), devRequestInst)
	if err != nil {
		t.Errorf("Delete devClaimInst: (%v)", err)
	}

	res, err := r.Reconcile(req)
	if err != nil {
		t.Logf("reconcile: (%v)", err)
	}

	// Check the result of reconciliation to make sure it has the desired state.
	if !res.Requeue {
		t.Log("reconcile did not requeue request as expected")
	}

	dvRequestInst := &openebsv1alpha1.BlockDeviceClaim{}
	err = r.client.Get(context.TODO(), req.NamespacedName, dvRequestInst)
	if errors.IsNotFound(err) {
		t.Logf("BlockDeviceClaim is deleted, expected")
		err = nil
	} else if err != nil {
		t.Errorf("Get dvClaimInst: (%v)", err)
	}

	time.Sleep(10 * time.Second)
	// Fetch the BlockDevice CR
	devInst := &openebsv1alpha1.BlockDevice{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: deviceName, Namespace: namespace}, devInst)
	if err != nil {
		t.Errorf("get devInst: (%v)", err)
	}

	if devInst.Spec.ClaimRef.UID == dvRequestInst.ObjectMeta.UID {
		t.Logf("BlockDevice ObjRef UID:%v match expected deviceRequest UID:%v",
			devInst.Spec.ClaimRef.UID, dvRequestInst.ObjectMeta.UID)
	} else {
		t.Fatalf("BlockDevice ObjRef UID:%v did not match expected deviceRequest UID:%v",
			devInst.Spec.ClaimRef.UID, dvRequestInst.ObjectMeta.UID)
	}

	if devInst.Status.ClaimState == openebsv1alpha1.BlockDeviceClaimed {
		t.Logf("BlockDevice Obj state:%v match expected state:%v",
			devInst.Status.ClaimState, openebsv1alpha1.BlockDeviceClaimed)
	} else {
		t.Fatalf("BlockDevice Obj state:%v did not match expected state:%v",
			devInst.Status.ClaimState, openebsv1alpha1.BlockDeviceClaimed)
	}
}

func (r *ReconcileBlockDeviceClaim) DeviceRequestedHappyPathTest(t *testing.T,
	req reconcile.Request) {

	devRequestInst := &openebsv1alpha1.BlockDeviceClaim{}
	// Fetch the BlockDeviceClaim CR
	err := r.client.Get(context.TODO(), req.NamespacedName, devRequestInst)
	if err != nil {
		t.Errorf("Get devRequestInst: (%v)", err)
	}

	// Fetch the BlockDevice CR
	devInst := &openebsv1alpha1.BlockDevice{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: deviceName, Namespace: namespace}, devInst)
	if err != nil {
		t.Errorf("get devInst: (%v)", err)
	}

	if devInst.Spec.ClaimRef.UID == devRequestInst.ObjectMeta.UID {
		t.Logf("BlockDevice ObjRef UID:%v match expected deviceRequest UID:%v",
			devInst.Spec.ClaimRef.UID, devRequestInst.ObjectMeta.UID)
	} else {
		t.Fatalf("BlockDevice ObjRef UID:%v did not match expected deviceRequest UID:%v",
			devInst.Spec.ClaimRef.UID, devRequestInst.ObjectMeta.UID)
	}

	if devInst.Status.ClaimState == openebsv1alpha1.BlockDeviceClaimed {
		t.Logf("BlockDevice Obj state:%v match expected state:%v",
			devInst.Status.ClaimState, openebsv1alpha1.BlockDeviceClaimed)
	} else {
		t.Fatalf("BlockDevice Obj state:%v did not match expected state:%v",
			devInst.Status.ClaimState, openebsv1alpha1.BlockDeviceClaimed)
	}
}

func (r *ReconcileBlockDeviceClaim) InvalidCapacityTest(t *testing.T,
	req reconcile.Request) {

	devRequestInst := &openebsv1alpha1.BlockDeviceClaim{}
	err := r.client.Get(context.TODO(), req.NamespacedName, devRequestInst)
	if err != nil {
		t.Errorf("Get devRequestInst: (%v)", err)
	}

	devRequestInst.Spec.Requirements.Requests[openebsv1alpha1.ResourceCapacity] = resource.MustParse("0")
	err = r.client.Update(context.TODO(), devRequestInst)
	if err != nil {
		t.Errorf("Update devRequestInst: (%v)", err)
	}

	res, err := r.Reconcile(req)
	if err != nil {
		t.Logf("reconcile: (%v)", err)
	}

	// Check the result of reconciliation to make sure it has the desired state.
	if !res.Requeue {
		t.Log("reconcile did not requeue request as expected")
	}

	dvC := &openebsv1alpha1.BlockDeviceClaim{}
	err = r.client.Get(context.TODO(), req.NamespacedName, dvC)
	if errors.IsNotFound(err) {
		t.Logf("BlockDeviceClaim is deleted, expected")
		err = nil
	} else if err != nil {
		t.Errorf("Get devRequestInst: (%v)", err)
	}
}

func (r *ReconcileBlockDeviceClaim) CheckBlockDeviceClaimStatus(t *testing.T,
	req reconcile.Request, phase openebsv1alpha1.DeviceClaimPhase) {

	devRequestCR := &openebsv1alpha1.BlockDeviceClaim{}
	err := r.client.Get(context.TODO(), req.NamespacedName, devRequestCR)
	if err != nil {
		t.Errorf("get devRequestCR : (%v)", err)
	}

	// BlockDeviceClaim should yet to bound.
	if devRequestCR.Status.Phase == phase {
		t.Logf("BlockDeviceClaim Object status:%v match expected status:%v",
			devRequestCR.Status.Phase, phase)
	} else {
		t.Fatalf("BlockDeviceClaim Object status:%v did not match expected status:%v",
			devRequestCR.Status.Phase, phase)
	}
}

func GetFakeBlockDeviceClaimObject() *openebsv1alpha1.BlockDeviceClaim {
	deviceRequestCR := &openebsv1alpha1.BlockDeviceClaim{}

	TypeMeta := metav1.TypeMeta{
		Kind:       "BlockDeviceClaim",
		APIVersion: ndm.NDMVersion,
	}

	ObjectMeta := metav1.ObjectMeta{
		Labels:    make(map[string]string),
		Name:      blockDeviceClaimName,
		Namespace: namespace,
		UID:       blockDeviceClaimUID,
	}

	Requests := v1.ResourceList{openebsv1alpha1.ResourceCapacity: claimCapacity}

	Requirements := openebsv1alpha1.DeviceClaimRequirements{
		Requests: Requests,
	}

	Spec := openebsv1alpha1.DeviceClaimSpec{
		Requirements: Requirements,
		DeviceType:   "",
		HostName:     fakeHostName,
	}

	deviceRequestCR.ObjectMeta = ObjectMeta
	deviceRequestCR.TypeMeta = TypeMeta
	deviceRequestCR.Spec = Spec
	deviceRequestCR.Status.Phase = openebsv1alpha1.BlockDeviceClaimStatusEmpty
	return deviceRequestCR
}

func GetFakeDeviceObject(bdName string, bdCapacity uint64) *openebsv1alpha1.BlockDevice {
	device := &openebsv1alpha1.BlockDevice{}

	TypeMeta := metav1.TypeMeta{
		Kind:       ndm.NDMBlockDeviceKind,
		APIVersion: ndm.NDMVersion,
	}

	ObjectMeta := metav1.ObjectMeta{
		Labels:    make(map[string]string),
		Name:      bdName,
		Namespace: namespace,
	}

	Spec := openebsv1alpha1.DeviceSpec{
		Path: "dev/disk-fake-path",
		Capacity: openebsv1alpha1.DeviceCapacity{
			Storage: bdCapacity, // Set blockdevice size.
		},
		DevLinks:    make([]openebsv1alpha1.DeviceDevLink, 0),
		Partitioned: ndm.NDMNotPartitioned,
	}

	device.ObjectMeta = ObjectMeta
	device.TypeMeta = TypeMeta
	device.Status.ClaimState = openebsv1alpha1.BlockDeviceUnclaimed
	device.Status.State = ndm.NDMActive
	device.Spec = Spec
	return device
}

func GetFakeDiskObject() *openebsv1alpha1.Disk {

	disk := &openebsv1alpha1.Disk{
		TypeMeta: metav1.TypeMeta{
			Kind:       ndm.NDMDiskKind,
			APIVersion: ndm.NDMVersion,
		},

		ObjectMeta: metav1.ObjectMeta{
			Labels:    make(map[string]string),
			Name:      diskName,
			Namespace: namespace,
		},

		Spec: openebsv1alpha1.DiskSpec{
			Path: "dev/disk-fake-path",
			Capacity: openebsv1alpha1.DiskCapacity{
				Storage: capacity, // Set disk size.
			},
			Details: openebsv1alpha1.DiskDetails{
				Model:  "disk-fake-model",
				Serial: "disk-fake-serial",
				Vendor: "disk-fake-vendor",
			},
			DevLinks: make([]openebsv1alpha1.DiskDevLink, 0),
		},
		Status: openebsv1alpha1.DiskStatus{
			State: ndm.NDMActive,
		},
	}
	disk.ObjectMeta.Labels[ndm.NDMHostKey] = fakeHostName
	return disk
}

func CreateFakeClient(t *testing.T) (client.Client, *runtime.Scheme) {

	diskR := GetFakeDiskObject()

	diskList := &openebsv1alpha1.DiskList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Disk",
			APIVersion: "",
		},
	}

	deviceR := GetFakeDeviceObject(deviceName, capacity)

	deviceList := &openebsv1alpha1.BlockDeviceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BlockDevice",
			APIVersion: "",
		},
	}

	deviceRequestCR := GetFakeBlockDeviceClaimObject()
	deviceclaimList := &openebsv1alpha1.BlockDeviceClaimList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BlockDeviceClaim",
			APIVersion: "",
		},
	}

	diskObjs := []runtime.Object{diskR}
	s := scheme.Scheme

	s.AddKnownTypes(openebsv1alpha1.SchemeGroupVersion, diskR)
	s.AddKnownTypes(openebsv1alpha1.SchemeGroupVersion, diskList)
	s.AddKnownTypes(openebsv1alpha1.SchemeGroupVersion, deviceR)
	s.AddKnownTypes(openebsv1alpha1.SchemeGroupVersion, deviceList)
	s.AddKnownTypes(openebsv1alpha1.SchemeGroupVersion, deviceRequestCR)
	s.AddKnownTypes(openebsv1alpha1.SchemeGroupVersion, deviceclaimList)

	fakeNdmClient := fake.NewFakeClient(diskObjs...)
	if fakeNdmClient == nil {
		fmt.Println("NDMClient is not created")
	}

	// Create a new blockdevice obj
	err := fakeNdmClient.Create(context.TODO(), deviceR)
	if err != nil {
		fmt.Println("BlockDevice object is not created")
	}

	// Create a new deviceclaim obj
	err = fakeNdmClient.Create(context.TODO(), deviceRequestCR)
	if err != nil {
		fmt.Println("BlockDeviceClaim object is not created")
	}
	return fakeNdmClient, s
}

func TestSelectBlockDevice(t *testing.T) {
	bdList := openebsv1alpha1.BlockDeviceList{}
	bd1 := GetFakeDeviceObject("blockdevice-example1", 102400)
	bd2 := GetFakeDeviceObject("blockdevice-example1", 1024000)
	bdList.Items = append(bdList.Items, *bd1, *bd2)

	resourceList1 := v1.ResourceList{openebsv1alpha1.ResourceCapacity: resource.MustParse("102400")}
	resourceList2 := v1.ResourceList{openebsv1alpha1.ResourceCapacity: resource.MustParse("2048000")}

	tests := map[string]struct {
		deviceList     openebsv1alpha1.BlockDeviceList
		rList          v1.ResourceList
		expectedDevice openebsv1alpha1.BlockDevice
		expectedOk     bool
	}{
		"can find a block device with matching requirements":    {bdList, resourceList1, *bd1, true},
		"cannot find a block device with matching requirements": {bdList, resourceList2, openebsv1alpha1.BlockDevice{}, false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			bd, ok := selectBlockDevice(test.deviceList, test.rList)
			assert.Equal(t, test.expectedDevice, bd)
			assert.Equal(t, test.expectedOk, ok)
		})
	}
}

func TestMatchResourceRequirements(t *testing.T) {
	blockDevice := GetFakeDeviceObject(deviceName, capacity)
	tests := map[string]struct {
		blockDevice *openebsv1alpha1.BlockDevice
		rList       v1.ResourceList
		expected    bool
	}{
		"block device capacity greater than requested capacity": {blockDevice,
			v1.ResourceList{openebsv1alpha1.ResourceCapacity: resource.MustParse("1024000")},
			true},
		"block device capacity is less than requested capacity": {blockDevice,
			v1.ResourceList{openebsv1alpha1.ResourceCapacity: resource.MustParse("404800000")},
			false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, matchResourceRequirements(*test.blockDevice, test.rList))
		})
	}
}