#!/usr/bin/env python3

import json
import requests
import copy

class FrostImportUtils:
    """
    Utility functions for importing observations into Frost
    """

    @staticmethod
    def __remove_observations(dset):
        dset2 = copy.deepcopy(dset)
        for ts in dset2['tseries']:
            ts['observations'] = []
        return dset2

    @staticmethod
    def delete_timeseries(frost_api_base, dset):
        url = '{}/api/v1/obs/badevann/ts/delete'.format(frost_api_base)

        # remove/empty all 'observations' objects from the dataset
        dset2 = FrostImportUtils.__remove_observations(dset)

        headers = {
            "X-Frost-Writetoken": "GOM_L-5dtRsaK26_TnaMnLz45vpcQzawmTYB1Kbp-jfD"
        }

        print('deleting time series in Frost/badevann ({}) ...'.format(url))
        r = requests.post(url, headers=headers, json=dset2)

        if r.status_code != 200:
            try:
                json_content = r.json()
            except:
                json_content = '((failed to extract json content))'
            raise Exception('request failed with status code {}: {}'.format(
                r.status_code, json_content))

    @staticmethod
    def create_timeseries(frost_api_base, dset):
        url = '{}/api/v1/obs/badevann/ts/create'.format(frost_api_base)

        # remove/empty all 'observations' objects from the dataset
        dset2 = FrostImportUtils.__remove_observations(dset)

        headers = {
            "X-Frost-Writetoken": "GOM_L-5dtRsaK26_TnaMnLz45vpcQzawmTYB1Kbp-jfD"
        }

        print('creating time series in Frost/badevann ({}) ...'.format(url))
        print(json.dumps(dset2))

        r = requests.post(url, headers=headers, json=dset2)

        if r.status_code != 200:
            try:
                json_content = r.json()
            except:
                json_content = '((failed to extract json content))'
            raise Exception('request failed with status code {}: {}'.format(
                r.status_code, json_content))

    @staticmethod
    def upload_dataset(frost_api_base, dset):
        url = '{}/api/v1/obs/badevann/put'.format(frost_api_base)

        #XXX: Read from token file or environment variable
        headers = {
            "X-Frost-Writetoken": "GOM_L-5dtRsaK26_TnaMnLz45vpcQzawmTYB1Kbp-jfD"
        }

        print('uploading to Frost/badevann ({}) ...'.format(url))
        r = requests.post(url, headers=headers, json=dset)

        if r.status_code != 200:
            try:
                json_content = r.json()
            except:
                json_content = '((failed to extract json content))'
            raise Exception('request failed with status code {}: {}'.format(
                r.status_code, json_content))

    @staticmethod
    def create_badevann_dataset(buoy_id, parameter, name, lon, lat, source, observations, year=None, month=None):

        dset = {
            "tstype": "badevann",
            "tseries": [
                {
                    "header": {
                        "id": {
                            "buoyID": str(buoy_id),
                            "parameter": parameter,
                            "source": source
                        },
                        "extra": {
                            "name": name,
                            "pos": {
                                "lon": str(lon),
                                "lat": str(lat)
                            }
                        }
                    },
                    "observations": []
                }
            ]
        }
       
        frost_observations = []
        for observation in observations:
            if year is None or observation[0][0:4] == year:
                if month is None or observation[0][5:7] == month:
                    frost_observations.append(
                            {
                                "time": observation[0],
                                "body": {
                                    "value": str(observation[1])
                                }
                            })

        dset['tseries'][0]['observations'] = frost_observations

        return dset
