package hcloud

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/hetznercloud/hcloud-go/v2/hcloud/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/imamik/k8zner/internal/config"
)

// testClientMinimal creates a RealClient with test timeouts and no hcloud.Client.
// Use this for tests where waitForActions/waitForActionResult are not reached
// (e.g., nil/empty actions, error paths before action waiting).
func testClientMinimal() *RealClient {
	return &RealClient{
		timeouts: config.TestTimeouts(),
	}
}

// --- DeleteOperation ---

func TestDeleteOperation_ResourceExists(t *testing.T) {
	t.Parallel()

	fw := &hcloud.Firewall{ID: 1, Name: "test-fw"}
	deleteCalled := false

	op := &DeleteOperation[*hcloud.Firewall]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, name string) (*hcloud.Firewall, *hcloud.Response, error) {
			assert.Equal(t, "test-fw", name)
			return fw, nil, nil
		},
		Delete: func(_ context.Context, resource *hcloud.Firewall) (*hcloud.Response, error) {
			deleteCalled = true
			assert.Equal(t, fw, resource)
			return nil, nil
		},
	}

	err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
	assert.True(t, deleteCalled, "Delete should have been called")
}

func TestDeleteOperation_ResourceNotFound(t *testing.T) {
	t.Parallel()

	op := &DeleteOperation[*hcloud.Firewall]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return nil, nil, nil
		},
		Delete: func(_ context.Context, _ *hcloud.Firewall) (*hcloud.Response, error) {
			t.Fatal("Delete should not be called for non-existent resource")
			return nil, nil
		},
	}

	err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
}

func TestDeleteOperation_GetError(t *testing.T) {
	t.Parallel()

	op := &DeleteOperation[*hcloud.Firewall]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return nil, nil, errors.New("API error")
		},
		Delete: func(_ context.Context, _ *hcloud.Firewall) (*hcloud.Response, error) {
			t.Fatal("Delete should not be called when Get fails")
			return nil, nil
		},
	}

	err := op.Execute(context.Background(), testClientMinimal())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get firewall")
	assert.Contains(t, err.Error(), "API error")
}

func TestDeleteOperation_DeleteError(t *testing.T) {
	t.Parallel()

	fw := &hcloud.Firewall{ID: 1, Name: "test-fw"}

	op := &DeleteOperation[*hcloud.Firewall]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return fw, nil, nil
		},
		Delete: func(_ context.Context, _ *hcloud.Firewall) (*hcloud.Response, error) {
			return nil, errors.New("delete failed")
		},
	}

	err := op.Execute(context.Background(), testClientMinimal())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

func TestDeleteOperation_LockedResourceRetried(t *testing.T) {
	t.Parallel()

	fw := &hcloud.Firewall{ID: 1, Name: "test-fw"}
	attempts := 0

	op := &DeleteOperation[*hcloud.Firewall]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return fw, nil, nil
		},
		Delete: func(_ context.Context, _ *hcloud.Firewall) (*hcloud.Response, error) {
			attempts++
			if attempts < 2 {
				return nil, hcloud.Error{Code: hcloud.ErrorCodeLocked, Message: "resource is locked"}
			}
			return nil, nil
		},
	}

	err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
	assert.Equal(t, 2, attempts, "should have retried after locked error")
}

func TestDeleteOperation_AllLockedErrorCodes(t *testing.T) {
	t.Parallel()

	lockedCodes := []struct {
		name string
		code hcloud.ErrorCode
	}{
		{"locked", hcloud.ErrorCodeLocked},
		{"conflict", hcloud.ErrorCodeConflict},
		{"resource_locked", hcloud.ErrorCodeResourceLocked},
		{"resource_unavailable", hcloud.ErrorCodeResourceUnavailable},
	}

	for _, tc := range lockedCodes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fw := &hcloud.Firewall{ID: 1, Name: "test-fw"}
			attempts := 0

			op := &DeleteOperation[*hcloud.Firewall]{
				Name:         "test-fw",
				ResourceType: "firewall",
				Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
					return fw, nil, nil
				},
				Delete: func(_ context.Context, _ *hcloud.Firewall) (*hcloud.Response, error) {
					attempts++
					if attempts < 2 {
						return nil, hcloud.Error{Code: tc.code, Message: "locked"}
					}
					return nil, nil
				},
			}

			err := op.Execute(context.Background(), testClientMinimal())
			require.NoError(t, err)
			assert.GreaterOrEqual(t, attempts, 2, "error code %s should trigger retry", tc.code)
		})
	}
}

func TestDeleteOperation_LockedResourceExhausted(t *testing.T) {
	t.Parallel()

	fw := &hcloud.Firewall{ID: 1, Name: "test-fw"}

	op := &DeleteOperation[*hcloud.Firewall]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return fw, nil, nil
		},
		Delete: func(_ context.Context, _ *hcloud.Firewall) (*hcloud.Response, error) {
			return nil, hcloud.Error{Code: hcloud.ErrorCodeLocked, Message: "resource is locked"}
		},
	}

	err := op.Execute(context.Background(), testClientMinimal())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation failed after")
}

// --- EnsureOperation ---

func TestEnsureOperation_CreateNew(t *testing.T) {
	t.Parallel()

	expectedFW := &hcloud.Firewall{ID: 1, Name: "new-fw"}
	createCalled := false

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, any]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, name string) (*hcloud.Firewall, *hcloud.Response, error) {
			assert.Equal(t, "test-fw", name)
			return nil, nil, nil
		},
		Create: func(_ context.Context, opts hcloud.FirewallCreateOpts) (*CreateResult[*hcloud.Firewall], *hcloud.Response, error) {
			createCalled = true
			assert.Equal(t, "test-fw", opts.Name)
			return &CreateResult[*hcloud.Firewall]{Resource: expectedFW}, nil, nil
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{Name: "test-fw"}
		},
	}

	result, err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
	assert.Equal(t, expectedFW, result)
	assert.True(t, createCalled, "Create should have been called")
}

func TestEnsureOperation_ReturnExisting(t *testing.T) {
	t.Parallel()

	existingFW := &hcloud.Firewall{ID: 42, Name: "existing-fw"}

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, any]{
		Name:         "existing-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return existingFW, nil, nil
		},
		Create: func(_ context.Context, _ hcloud.FirewallCreateOpts) (*CreateResult[*hcloud.Firewall], *hcloud.Response, error) {
			t.Fatal("Create should not be called when resource exists")
			return nil, nil, nil
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	result, err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
	assert.Equal(t, existingFW, result)
}

func TestEnsureOperation_ExistingWithValidation(t *testing.T) {
	t.Parallel()

	existingFW := &hcloud.Firewall{ID: 42, Name: "valid-fw"}
	validateCalled := false

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, any]{
		Name:         "valid-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return existingFW, nil, nil
		},
		Validate: func(fw *hcloud.Firewall) error {
			validateCalled = true
			if fw.Name != "valid-fw" {
				return errors.New("name mismatch")
			}
			return nil
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	result, err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
	assert.Equal(t, existingFW, result)
	assert.True(t, validateCalled, "Validate should have been called")
}

func TestEnsureOperation_ValidationFails(t *testing.T) {
	t.Parallel()

	existingFW := &hcloud.Firewall{ID: 42, Name: "wrong-fw"}

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, any]{
		Name:         "wrong-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return existingFW, nil, nil
		},
		Validate: func(_ *hcloud.Firewall) error {
			return errors.New("validation failed: IP range mismatch")
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	_, err := op.Execute(context.Background(), testClientMinimal())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestEnsureOperation_ExistingWithUpdate(t *testing.T) {
	t.Parallel()

	existingFW := &hcloud.Firewall{ID: 42, Name: "update-fw"}
	updateCalled := false

	type updateOpts struct {
		Rules []string
	}

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, updateOpts]{
		Name:         "update-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return existingFW, nil, nil
		},
		Update: func(_ context.Context, fw *hcloud.Firewall, opts updateOpts) ([]*hcloud.Action, *hcloud.Response, error) {
			updateCalled = true
			assert.Equal(t, existingFW, fw)
			assert.Equal(t, []string{"allow-http"}, opts.Rules)
			return nil, nil, nil // nil actions → waitForActions short-circuits
		},
		UpdateOptsMapper: func(_ *hcloud.Firewall) updateOpts {
			return updateOpts{Rules: []string{"allow-http"}}
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	result, err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
	assert.Equal(t, existingFW, result)
	assert.True(t, updateCalled, "Update should have been called")
}

func TestEnsureOperation_ValidateBeforeUpdate(t *testing.T) {
	t.Parallel()

	existingFW := &hcloud.Firewall{ID: 42, Name: "fw"}
	callOrder := []string{}

	type updateOpts struct{}

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, updateOpts]{
		Name:         "fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return existingFW, nil, nil
		},
		Validate: func(_ *hcloud.Firewall) error {
			callOrder = append(callOrder, "validate")
			return nil
		},
		Update: func(_ context.Context, _ *hcloud.Firewall, _ updateOpts) ([]*hcloud.Action, *hcloud.Response, error) {
			callOrder = append(callOrder, "update")
			return nil, nil, nil
		},
		UpdateOptsMapper: func(_ *hcloud.Firewall) updateOpts {
			return updateOpts{}
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	_, err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
	assert.Equal(t, []string{"validate", "update"}, callOrder, "validate should run before update")
}

func TestEnsureOperation_ValidationBlocksUpdate(t *testing.T) {
	t.Parallel()

	existingFW := &hcloud.Firewall{ID: 42, Name: "fw"}

	type updateOpts struct{}

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, updateOpts]{
		Name:         "fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return existingFW, nil, nil
		},
		Validate: func(_ *hcloud.Firewall) error {
			return errors.New("validation failed")
		},
		Update: func(_ context.Context, _ *hcloud.Firewall, _ updateOpts) ([]*hcloud.Action, *hcloud.Response, error) {
			t.Fatal("Update should not be called when validation fails")
			return nil, nil, nil
		},
		UpdateOptsMapper: func(_ *hcloud.Firewall) updateOpts {
			return updateOpts{}
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	_, err := op.Execute(context.Background(), testClientMinimal())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestEnsureOperation_UpdateSkippedWithoutMapper(t *testing.T) {
	t.Parallel()

	existingFW := &hcloud.Firewall{ID: 42, Name: "fw"}

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, any]{
		Name:         "fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return existingFW, nil, nil
		},
		Update: func(_ context.Context, _ *hcloud.Firewall, _ any) ([]*hcloud.Action, *hcloud.Response, error) {
			t.Fatal("Update should not be called without UpdateOptsMapper")
			return nil, nil, nil
		},
		// UpdateOptsMapper intentionally nil
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	result, err := op.Execute(context.Background(), testClientMinimal())
	require.NoError(t, err)
	assert.Equal(t, existingFW, result)
}

func TestEnsureOperation_GetError(t *testing.T) {
	t.Parallel()

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, any]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return nil, nil, errors.New("API error")
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	_, err := op.Execute(context.Background(), testClientMinimal())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get firewall")
	assert.Contains(t, err.Error(), "API error")
}

func TestEnsureOperation_CreateError(t *testing.T) {
	t.Parallel()

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, any]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return nil, nil, nil
		},
		Create: func(_ context.Context, _ hcloud.FirewallCreateOpts) (*CreateResult[*hcloud.Firewall], *hcloud.Response, error) {
			return nil, nil, errors.New("create failed")
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	_, err := op.Execute(context.Background(), testClientMinimal())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create firewall")
	assert.Contains(t, err.Error(), "create failed")
}

func TestEnsureOperation_UpdateError(t *testing.T) {
	t.Parallel()

	existingFW := &hcloud.Firewall{ID: 42, Name: "test-fw"}

	type updateOpts struct{}

	op := &EnsureOperation[*hcloud.Firewall, hcloud.FirewallCreateOpts, updateOpts]{
		Name:         "test-fw",
		ResourceType: "firewall",
		Get: func(_ context.Context, _ string) (*hcloud.Firewall, *hcloud.Response, error) {
			return existingFW, nil, nil
		},
		Update: func(_ context.Context, _ *hcloud.Firewall, _ updateOpts) ([]*hcloud.Action, *hcloud.Response, error) {
			return nil, nil, errors.New("update failed")
		},
		UpdateOptsMapper: func(_ *hcloud.Firewall) updateOpts {
			return updateOpts{}
		},
		CreateOptsMapper: func() hcloud.FirewallCreateOpts {
			return hcloud.FirewallCreateOpts{}
		},
	}

	_, err := op.Execute(context.Background(), testClientMinimal())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update firewall")
	assert.Contains(t, err.Error(), "update failed")
}

// --- waitForActions ---

func TestWaitForActions_NoActions(t *testing.T) {
	t.Parallel()

	// nil client is safe because no actions means no API call
	err := waitForActions(context.Background(), nil)
	require.NoError(t, err)
}

func TestWaitForActions_EmptySlice(t *testing.T) {
	t.Parallel()

	actions := []*hcloud.Action{}
	err := waitForActions(context.Background(), nil, actions...)
	require.NoError(t, err)
}

func TestWaitForActions_WithAction(t *testing.T) {
	t.Parallel()

	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/actions/1", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 1, Status: "success", Progress: 100},
		})
	})

	action := &hcloud.Action{ID: 1}
	err := waitForActions(context.Background(), ts.client(), action)
	require.NoError(t, err)
}

func TestWaitForActions_MultipleActions(t *testing.T) {
	t.Parallel()

	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/actions", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionListResponse{
			Actions: []schema.Action{
				{ID: 1, Status: "success", Progress: 100},
				{ID: 2, Status: "success", Progress: 100},
			},
		})
	})

	actions := []*hcloud.Action{{ID: 1}, {ID: 2}}
	err := waitForActions(context.Background(), ts.client(), actions...)
	require.NoError(t, err)
}

// --- waitForActionResult ---

func TestWaitForActionResult_NoActions(t *testing.T) {
	t.Parallel()

	result := &CreateResult[*hcloud.Firewall]{
		Resource: &hcloud.Firewall{ID: 1},
	}
	err := waitForActionResult(context.Background(), nil, result)
	require.NoError(t, err)
}

func TestWaitForActionResult_EmptyActionsSlice(t *testing.T) {
	t.Parallel()

	result := &CreateResult[*hcloud.Firewall]{
		Resource: &hcloud.Firewall{ID: 1},
		Actions:  []*hcloud.Action{},
	}
	err := waitForActionResult(context.Background(), nil, result)
	require.NoError(t, err)
}

func TestWaitForActionResult_SingleAction(t *testing.T) {
	t.Parallel()

	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/actions/10", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 10, Status: "success", Progress: 100},
		})
	})

	result := &CreateResult[*hcloud.Firewall]{
		Resource: &hcloud.Firewall{ID: 1},
		Action:   &hcloud.Action{ID: 10},
	}
	err := waitForActionResult(context.Background(), ts.client(), result)
	require.NoError(t, err)
}

func TestWaitForActionResult_MultipleActions(t *testing.T) {
	t.Parallel()

	ts := newTestServer()
	defer ts.close()

	ts.handleFunc("/actions", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionListResponse{
			Actions: []schema.Action{
				{ID: 10, Status: "success", Progress: 100},
				{ID: 11, Status: "success", Progress: 100},
			},
		})
	})

	result := &CreateResult[*hcloud.Firewall]{
		Resource: &hcloud.Firewall{ID: 1},
		Actions:  []*hcloud.Action{{ID: 10}, {ID: 11}},
	}
	err := waitForActionResult(context.Background(), ts.client(), result)
	require.NoError(t, err)
}

func TestWaitForActionResult_SingleActionTakesPrecedence(t *testing.T) {
	t.Parallel()

	ts := newTestServer()
	defer ts.close()

	// Only set up handler for the single Action, not the Actions slice.
	// If code incorrectly uses Actions instead, this test will fail.
	ts.handleFunc("/actions/10", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, schema.ActionGetResponse{
			Action: schema.Action{ID: 10, Status: "success", Progress: 100},
		})
	})

	result := &CreateResult[*hcloud.Firewall]{
		Resource: &hcloud.Firewall{ID: 1},
		Action:   &hcloud.Action{ID: 10},
		Actions:  []*hcloud.Action{{ID: 20}}, // should be ignored
	}
	err := waitForActionResult(context.Background(), ts.client(), result)
	require.NoError(t, err)
}

// --- simpleCreate ---

func TestSimpleCreate_Success(t *testing.T) {
	t.Parallel()

	type createOpts struct{ Name string }

	createFn := func(_ context.Context, opts createOpts) (*hcloud.Network, *hcloud.Response, error) {
		return &hcloud.Network{ID: 42, Name: opts.Name}, nil, nil
	}

	wrapped := simpleCreate(createFn)

	result, resp, err := wrapped(context.Background(), createOpts{Name: "test-net"})
	require.NoError(t, err)
	assert.Nil(t, resp)
	require.NotNil(t, result)
	assert.Equal(t, int64(42), result.Resource.ID)
	assert.Equal(t, "test-net", result.Resource.Name)
	assert.Nil(t, result.Action, "simpleCreate should not set Action")
	assert.Nil(t, result.Actions, "simpleCreate should not set Actions")
}

func TestSimpleCreate_Error(t *testing.T) {
	t.Parallel()

	type createOpts struct{ Name string }

	createFn := func(_ context.Context, _ createOpts) (*hcloud.Network, *hcloud.Response, error) {
		return nil, nil, errors.New("create failed")
	}

	wrapped := simpleCreate(createFn)

	result, _, err := wrapped(context.Background(), createOpts{Name: "test"})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "create failed")
}

func TestSimpleCreate_PreservesResponse(t *testing.T) {
	t.Parallel()

	type createOpts struct{}

	resp := &hcloud.Response{}
	createFn := func(_ context.Context, _ createOpts) (*hcloud.Network, *hcloud.Response, error) {
		return &hcloud.Network{ID: 1}, resp, nil
	}

	wrapped := simpleCreate(createFn)

	result, gotResp, err := wrapped(context.Background(), createOpts{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, resp, gotResp, "response should be passed through")
}

// --- CreateResult ---

func TestCreateResult_Fields(t *testing.T) {
	t.Parallel()

	action := &hcloud.Action{ID: 1}
	actions := []*hcloud.Action{{ID: 2}, {ID: 3}}
	fw := &hcloud.Firewall{ID: 42}

	result := &CreateResult[*hcloud.Firewall]{
		Resource: fw,
		Action:   action,
		Actions:  actions,
	}

	assert.Equal(t, fw, result.Resource)
	assert.Equal(t, action, result.Action)
	assert.Equal(t, actions, result.Actions)
}
