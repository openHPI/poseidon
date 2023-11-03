package api

import (
	"encoding/json"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"io"
	"net/http"
	"net/http/httptest"
)

func (s *MainTestSuite) TestHealth() {
	s.Run("returns StatusNoContent as default", func() {
		request, err := http.NewRequest(http.MethodGet, "/health", http.NoBody)
		if err != nil {
			s.T().Fatal(err)
		}
		recorder := httptest.NewRecorder()
		manager := &environment.ManagerHandlerMock{}
		manager.On("Statistics").Return(map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData{})

		Health(manager).ServeHTTP(recorder, request)
		s.Equal(http.StatusNoContent, recorder.Code)
	})
	s.Run("returns InternalServerError for warnings and errors", func() {
		s.Run("Prewarming Pool Alert", func() {
			request, err := http.NewRequest(http.MethodGet, "/health", http.NoBody)
			if err != nil {
				s.T().Fatal(err)
			}
			recorder := httptest.NewRecorder()
			manager := &environment.ManagerHandlerMock{}
			manager.On("Statistics").Return(map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData{
				tests.DefaultEnvironmentIDAsInteger: {
					ID:                 tests.DefaultEnvironmentIDAsInteger,
					PrewarmingPoolSize: 3,
					IdleRunners:        1,
				},
			})
			config.Config.Server.Alert.PrewarmingPoolThreshold = 0.5

			Health(manager).ServeHTTP(recorder, request)
			s.Equal(http.StatusServiceUnavailable, recorder.Code)

			b, err := io.ReadAll(recorder.Body)
			s.Require().NoError(err)
			var details dto.InternalServerError
			err = json.Unmarshal(b, &details)
			s.Require().NoError(err)
			s.Contains(details.Message, ErrorPrewarmingPoolDepleting.Error())
		})
	})
}
