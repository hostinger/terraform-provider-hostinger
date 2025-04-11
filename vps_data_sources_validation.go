package main

import (
    "fmt"
    "encoding/json"
    "net/http"
    "io"
)

// ValidatePlanID checks if the provided plan exists in /billing/v1/catalog
func (c *HostingerClient) ValidatePlanID(plan string) (bool, error) {
    url := c.BaseURL + "/api/billing/v1/catalog"
    req, _ := http.NewRequest("GET", url, nil)
    c.addStandardHeaders(req)

    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        msg, _ := io.ReadAll(resp.Body)
        return false, fmt.Errorf("failed to list plans (HTTP %d): %s", resp.StatusCode, msg)
    }

    var catalog []struct {
        Prices []struct {
            ID string `json:"id"`
        } `json:"prices"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
        return false, err
    }

    for _, item := range catalog {
        for _, price := range item.Prices {
            if price.ID == plan {
                return true, nil
            }
        }
    }
    return false, nil
}

// ValidateTemplateID checks if a template ID exists
func (c *HostingerClient) ValidateTemplateID(id int) (bool, error) {
    url := c.BaseURL + "/api/vps/v1/templates"
    req, _ := http.NewRequest("GET", url, nil)
    c.addStandardHeaders(req)

    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        msg, _ := io.ReadAll(resp.Body)
        return false, fmt.Errorf("failed to list templates (HTTP %d): %s", resp.StatusCode, msg)
    }

    var templates []struct {
        ID int `json:"id"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&templates); err != nil {
        return false, err
    }

    for _, t := range templates {
        if t.ID == id {
            return true, nil
        }
    }
    return false, nil
}

// ValidateDataCenterID checks if a data center ID exists
func (c *HostingerClient) ValidateDataCenterID(id int) (bool, error) {
    url := c.BaseURL + "/api/vps/v1/data-centers"
    req, _ := http.NewRequest("GET", url, nil)
    c.addStandardHeaders(req)

    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        msg, _ := io.ReadAll(resp.Body)
        return false, fmt.Errorf("failed to list data centers (HTTP %d): %s", resp.StatusCode, msg)
    }

    var datacenters []struct {
        ID int `json:"id"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&datacenters); err != nil {
        return false, err
    }

    for _, dc := range datacenters {
        if dc.ID == id {
            return true, nil
        }
    }
    return false, nil
}
