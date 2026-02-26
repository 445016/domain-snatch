package whois

import (
	"fmt"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/domain"
)

type AliyunClient struct {
	AccessKeyID     string
	AccessKeySecret string
	client          *domain.Client
}

func NewAliyunClient(accessKeyID, accessKeySecret string) (*AliyunClient, error) {
	client, err := domain.NewClientWithAccessKey("cn-hangzhou", accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("create aliyun client failed: %w", err)
	}

	return &AliyunClient{
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
		client:          client,
	}, nil
}

// QueryDomain 使用阿里云API查询域名信息
func (c *AliyunClient) QueryDomain(domainName string) (*Result, error) {
	result := &Result{
		Domain: domainName,
		Status: "unknown",
	}

	request := domain.CreateQueryDomainByDomainNameRequest()
	request.Scheme = "https"
	request.DomainName = domainName

	response, err := c.client.QueryDomainByDomainName(request)
	if err != nil {
		// 检查是否是域名未注册
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "not exist") || 
		   strings.Contains(errMsg, "not found") ||
		   strings.Contains(errMsg, "available") {
			result.Status = "available"
			result.CanRegister = true
			return result, nil
		}
		return result, fmt.Errorf("aliyun api query failed: %w", err)
	}

	// 解析响应
	result.Status = "registered"
	result.Registrar = response.RegistrantOrganization

	// 解析到期日期
	if response.ExpirationDate != "" {
		// 阿里云返回的时间格式：2027-07-30 00:00:00
		expiryTime, err := time.Parse("2006-01-02 15:04:05", response.ExpirationDate)
		if err != nil {
			expiryTime, err = time.Parse("2006-01-02", response.ExpirationDate)
		}
		if err == nil {
			result.ExpiryDate = &expiryTime
			if expiryTime.Before(time.Now()) {
				result.Status = "expired"
				result.CanRegister = true
			} else {
				result.CanRegister = false
			}
		}
	}

	// 解析注册日期
	if response.RegistrationDate != "" {
		creationTime, err := time.Parse("2006-01-02 15:04:05", response.RegistrationDate)
		if err != nil {
			creationTime, err = time.Parse("2006-01-02", response.RegistrationDate)
		}
		if err == nil {
			result.CreationDate = &creationTime
		}
	}

	// 构建WHOIS原始信息（从API响应）
	result.WhoisRaw = fmt.Sprintf(`Domain: %s
Status: %s
Expiration Date: %s
Registration Date: %s
Registrant: %s
`,
		response.DomainName,
		response.DomainStatus,
		response.ExpirationDate,
		response.RegistrationDate,
		response.RegistrantOrganization)

	return result, nil
}

// QueryDomainAvailability 批量检查域名是否可注册（阿里云特有，速度快）
func (c *AliyunClient) QueryDomainAvailability(domainNames []string) (map[string]string, error) {
	request := domain.CreateCheckDomainRequest()
	request.Scheme = "https"
	request.DomainName = strings.Join(domainNames, ",")

	response, err := c.client.CheckDomain(request)
	if err != nil {
		return nil, fmt.Errorf("aliyun batch check failed: %w", err)
	}

	result := make(map[string]string)
	result[response.DomainName] = response.Avail

	return result, nil
}
