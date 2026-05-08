#!/usr/bin/env python3

"""
This script demonstrates how datasets can be uploaded/downloaded to/from the Frost service.
The time series type used in this example is 'frost0'.
"""

# pylint: disable=invalid-name
# pylint: disable=too-few-public-methods
# pylint: disable=too-many-arguments
# pylint: disable=too-many-locals


import argparse
import sys
import json
import datetime
import math
import random
import copy
from traceback import format_exc
import requests  # See https://requests.readthedocs.io
from dateutil import tz


class Client:
    """
    Main class.
    """

    def __init__(self):
        frost_api_base, tsid, t1, t2, td, pos = self.__parse_args()
        self.__dtformat = '%Y-%m-%dT%H:%M:%SZ'

        # generate a dataset
        dset_up = self.__create_dataset(tsid, t1, t2, td, pos)

        # create the time series in the dataset to Frost with a POST request
        self.__create_timeseries(frost_api_base, dset_up)

        # upload the dataset to Frost with a POST request
        self.__upload_dataset(frost_api_base, dset_up)

        # download the same dataset from Frost and verify that it is identical to the
        # original one
        dset_down = self.__download_dataset(frost_api_base, tsid, t1, t2)
        self.__compare_datasets(dset_up, dset_down)

    @staticmethod
    def __parse_args():
        parser = argparse.ArgumentParser(formatter_class=argparse.ArgumentDefaultsHelpFormatter)
        parser.add_argument(
            '--fab', required=False, dest='frost_api_base', default='http://localhost:8080',
            help='the Frost API base')
        parser.add_argument('--tsid', required=True, dest='tsid', help='time series id')
        parser.add_argument(
            '--t1', required=True, dest='t1', help='first time (format: YYYY-MM-DDThh:mm:ssZ)')
        parser.add_argument(
            '--t2', required=True, dest='t2', help='last time (format: YYYY-MM-DDThh:mm:ssZ)')
        parser.add_argument('--td', required=True, dest='td', help='seconds between observations')
        parser.add_argument(
            '--pos', required=False, dest='pos', default='none', choices=['none', 'some', 'all'],
            help='optional (random) positions for individual observations')
        res = parser.parse_args(sys.argv[1:])
        return res.frost_api_base, res.tsid, res.t1, res.t2, res.td, res.pos

    def __parse_datetime(self, t):
        try:
            dt_obj = datetime.datetime.strptime(t, self.__dtformat)
        except ValueError:
            raise Exception('failed to parse {} as YYYY-MM-DDThh:mm: {}'.format(t, format_exc()))
        return int(dt_obj.replace(tzinfo=tz.tzutc()).timestamp())

    def __format_utcepoch(self, t):
        return datetime.datetime.utcfromtimestamp(t).strftime(self.__dtformat)

    @staticmethod
    def __parse_posint(s):
        try:
            i = int(s)
        except ValueError:
            raise Exception('failed to parse {} as integer: {}'.format(s, format_exc()))
        if i <= 0:
            raise Exception('not a positive integer: {}'.format(i))
        return i

    @staticmethod
    def __create_random_pos(pos):
        if pos == "none":
            return None
        rp = {'lon': str(random.randrange(-180, 180)), 'lat': str(random.randrange(-90, 90))}
        if pos == "all":
            return rp
        # pos == "some", so return random pos in around 50% of the calls
        if random.random() < 0.5:
            return rp
        return None

    # Creates a dataset with a single time series.
    def __create_dataset(self, tsid, t1, t2, td, pos):
        random.seed()  # ensure a different random sequence on each run

        tsid_obj = json.loads(tsid)

        dset = {
            'tstype': 'frost0',
            'tseries': [{
                'header': {
                    'id': tsid_obj,
                    'extra': None
                },
                'observations': None  # to be filled in below
            }]
        }

        observations = []

        t1secs = self.__parse_datetime(t1)
        t2secs = self.__parse_datetime(t2)
        if t1secs >= t2secs:
            raise Exception('{} is not earlier than {}'.format(t1, t2))

        tdsecs = self.__parse_posint(td)

        r = float(t2secs - t1secs)
        scale = 100
        _2pi = 2 * math.pi
        t = t1secs
        while t < t2secs:
            frac = (t - t1secs) / r
            val = scale * math.sin(frac * _2pi)
            val = int(100 * val) / 100  # round off to two decimal places
            obs_obj = {
                'time': '{}'.format(self.__format_utcepoch(t)),
                'body': {
                    'pos': self.__create_random_pos(pos),
                    'value': '{}'.format(val),
                    'quality': ''
                }
            }
            observations.append(obs_obj)
            t = t + tdsecs

        dset['tseries'][0]['observations'] = observations
        return dset

    @staticmethod
    def __remove_observations(dset):
        dset2 = copy.deepcopy(dset)
        for ts in dset2['tseries']:
            ts['observations'] = []
        return dset2

    def __create_timeseries(self, frost_api_base, dset):
        url = '{}/api/v1/obs/frost0/ts/create'.format(frost_api_base)

        # remove/empty all 'observations' objects from the dataset ... TBD
        dset2 = self.__remove_observations(dset)

        print('creating time series in Frost/frost0 ({}) ...'.format(url))
        r = requests.post(url, files={'dataset': json.dumps(dset2)})

        if r.status_code != 200:
            try:
                json_content = r.json()
            except:
                json_content = '((failed to extract json content))'
            raise Exception('request failed with status code {}: {}'.format(
                r.status_code, json_content))

    @staticmethod
    def __upload_dataset(frost_api_base, dset):
        url = '{}/api/v1/obs/frost0/put'.format(frost_api_base)

        print('uploading to Frost/frost0 ({}) ...'.format(url))
        r = requests.post(url, files={'dataset': json.dumps(dset)})

        if r.status_code != 200:
            try:
                json_content = r.json()
            except:
                json_content = '((failed to extract json content))'
            raise Exception('request failed with status code {}: {}'.format(
                r.status_code, json_content))

    @staticmethod
    def __download_dataset(frost_api_base, tsid, t1, t2):
        tsid_obj = json.loads(tsid)
        hdrmatch_obj = {
            'id': tsid_obj
        }
        hdrmatch = json.dumps(hdrmatch_obj)
        url = '{}/api/v1/obs/frost0/get?hdrmatch={}&time={}/{}&incobs=true'.format(
            frost_api_base, hdrmatch, t1, t2)

        print('downloading from Frost/frost0 ({}) ...'.format(url))
        r = requests.get(url)

        if r.status_code != 200:
            try:
                json_content = r.json()
            except:
                json_content = '((failed to extract json content))'
            raise Exception('request failed with status code {}: {}'.format(
                r.status_code, json_content))
        return r.json()['data']

    @staticmethod
    def __pretty_print(title, data):
        print(title)
        print(json.dumps(data, indent=4, separators=(',', ': ')))

    @staticmethod
    def __get_val_from_case_insensitive_key(d, k):
        k0 = k.lower()
        for key, val in d.items():
            if key.lower() == k0:
                return val
        return None  # not found

    def __header_ids_equivalent(self, id1, id2):
        if (id1 is None) or (id2 is None):
            return id1 is id2

        source1 = self.__get_val_from_case_insensitive_key(id1, 'source')
        source2 = self.__get_val_from_case_insensitive_key(id2, 'source')
        if source1 != source2:
            return False
        sensorLevel1 = self.__get_val_from_case_insensitive_key(id1, 'sensorLevel')
        sensorLevel2 = self.__get_val_from_case_insensitive_key(id2, 'sensorLevel')
        if sensorLevel1 != sensorLevel2:
            return False
        element1 = self.__get_val_from_case_insensitive_key(id1, 'element')
        element2 = self.__get_val_from_case_insensitive_key(id2, 'element')
        if element1 != element2:
            return False

        return True  # no inequivalences found

    def __header_extras_equivalent(self, extra1, extra2):
        if extra1 is None:
            return (extra2 is None) or (extra2 == {})
        if extra2 is None:
            return (extra1 is None) or (extra1 == {})

        # FOR NOW:
        if extra1 != extra2:
            return False

        return True  # no inequivalences found

    def __headers_equivalent(self, header1, header2):
        if (header1 is None) or (header2 is None):
            return header1 is header2

        id1 = self.__get_val_from_case_insensitive_key(header1, 'id')
        id2 = self.__get_val_from_case_insensitive_key(header2, 'id')
        if not self.__header_ids_equivalent(id1, id2):
            return False

        extra1 = self.__get_val_from_case_insensitive_key(header1, 'extra')
        extra2 = self.__get_val_from_case_insensitive_key(header2, 'extra')
        if not self.__header_extras_equivalent(extra1, extra2):
            return False

        return True  # no inequivalences found

    def __obs_bodies_equivalent(self, body1, body2):
        if (body1 is None) or (body2 is None):
            return body1 is body2

        p1 = self.__get_val_from_case_insensitive_key(body1, 'pos')
        p2 = self.__get_val_from_case_insensitive_key(body2, 'pos')
        if p1 != p2:
            return False
        v1 = self.__get_val_from_case_insensitive_key(body1, 'value')
        v2 = self.__get_val_from_case_insensitive_key(body2, 'value')
        if v1 != v2:
            return False
        q1 = self.__get_val_from_case_insensitive_key(body1, 'quality')
        q2 = self.__get_val_from_case_insensitive_key(body2, 'quality')
        if q1 != q2:
            return False

        return True  # no inequivalences found

    def __observations_equivalent(self, obs1, obs2):
        if len(obs1) != len(obs2):
            return False
        for i in range(len(obs1)):
            t1 = self.__get_val_from_case_insensitive_key(obs1[i], 'time')
            t2 = self.__get_val_from_case_insensitive_key(obs2[i], 'time')
            if t1 != t2:
                return False
            body1 = self.__get_val_from_case_insensitive_key(obs1[i], 'body')
            body2 = self.__get_val_from_case_insensitive_key(obs2[i], 'body')
            if not self.__obs_bodies_equivalent(body1, body2):
                return False

        return True  # no inequivalences found

    def __datasets_equivalent(self, dset1, dset2):
        tseries1 = dset1['tseries']
        tseries2 = dset2['tseries']
        if len(tseries1) != len(tseries2):
            return False
        for ts1, ts2 in zip(tseries1, tseries2):
            if not self.__headers_equivalent(ts1['header'], ts2['header']):
                return False
            if not self.__observations_equivalent(ts1['observations'], ts2['observations']):
                return False

        return True  # no inequivalences found

    def __compare_datasets(self, dset_up, dset_down):
        if dset_up == dset_down:
            print('datasets are identical')
        elif self.__datasets_equivalent(dset_up, dset_down):
            print(
                'datasets are equivalent, i.e. with some (unimportant) syntactical differences ' +
                'found wrt key/value pair order, amounts of external whitespace, ' +
                'None vs empty dict, object key case difference, etc.')
        else:
            print('datasets differ:')
            self.__pretty_print('uploaded dataset:', dset_up)
            print()
            self.__pretty_print('downloaded dataset:', dset_down)


if __name__ == "__main__":

    try:
        Client()
    except SystemExit as e:
        if e.code != 0:
            print('SystemExit(code={}): {}'.format(e.code, format_exc()), file=sys.stderr)
            sys.exit(e.code)
    except:  # pylint: disable=bare-except
        print('error: {}'.format(format_exc()), file=sys.stderr)
        sys.exit(1)

    sys.exit(0)
