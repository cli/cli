#!/usr/bin/python

import matplotlib
import matplotlib.pyplot as plt
from matplotlib import gridspec
import numpy as np
import datetime
import json
import pandas
import argparse
import os
matplotlib.style.use('ggplot')

# sacrificial plot for single legend
matplotlib.rcParams['figure.figsize'] = 1, 1
randFig = plt.figure()
randAx = plt.subplot()
randAx.plot(0, 0, color='green', label='good', marker='+')
randAx.plot(0, 0, color='red', label='failed', marker='x')
randAx.plot(0, 0, color='black', label='sent', linestyle='--')
randAx.plot(0, 0, color='green', label='50th quantile')
randAx.plot(0, 0, color='orange', label='90th quantile')
randAx.plot(0, 0, color='red', label='99th quantile')
handles, labels = randAx.get_legend_handles_labels()

# big ol' plotting method
def plot_section(all_data, title, outputPath):
    # group calls by the endpoint/method
    actions = all_data.groupby('action')
    h = len(actions.groups.keys())
    matplotlib.rcParams['figure.figsize'] = 20, 3 * h

    fig = plt.figure()
    fig.legend(handles, labels, ncol=6, fontsize=16, framealpha=0, loc='upper center')
    if title is not None:
        fig.suptitle(title, fontsize=20, y=0.93)
    gs = gridspec.GridSpec(h, 3)

    # figure out left and right datetime bounds
    started = all_data['sent'].min()
    stopped = all_data['finished'].max()

    i = 0
    # plot one row of charts for each endpoint/method combination
    for section in actions.groups.keys():
        # setup the tree charts
        ax = fig.add_subplot(gs[i, 0])
        ax.set_title(section)
        ax.set_xlim(started, stopped)
        ax2 = fig.add_subplot(gs[i, 2])
        ax2.set_xlim(started, stopped)
        ax3 = fig.add_subplot(gs[i, 1])
        ax3.set_xlim(started, stopped)

        # find the maximum y value and set it across all three charts
        calls = actions.get_group(section)
        tookMax = calls['took'].max()
        ax.set_ylim(0, tookMax+tookMax*0.1)
        ax2.set_ylim(0, tookMax+tookMax*0.1)
        ax3.set_ylim(0, tookMax+tookMax*0.1)

        groups = calls.groupby('type')
        if groups.groups.get('error', False) is not False:
            bad = groups.get_group('error')
            ax.plot_date(bad['finished'], bad['took'], color='red', marker='x', label='error')

            bad_rate = bad.set_index('finished')
            bad_rate['rate'] = [0] * len(bad_rate.index)
            bad_rate = bad_rate.resample('5S').count()
            bad_rate['rate'] = bad_rate['rate'].divide(5)
            rateMax = bad_rate['rate'].max()
            ax2.plot_date(bad_rate.index, bad_rate['rate'], linestyle='-', marker='', color='red', label='error')
        if groups.groups.get('good', False) is not False:
            good = groups.get_group('good')
            ax.plot_date(good['finished'], good['took'], color='green', marker='+', label='good')

            good_rate = good.set_index('finished')
            good_rate['rate'] = [0] * len(good_rate.index)
            good_rate = good_rate.resample('5S').count()
            good_rate['rate'] = good_rate['rate'].divide(5)
            rateMax = good_rate['rate'].max()
            ax2.plot_date(good_rate.index, good_rate['rate'], linestyle='-', marker='', color='green', label='good')
        ax.set_ylabel('Latency (ms)')

        # calculate the request rate
        sent_rate = pandas.DataFrame(calls['sent'])
        sent_rate = sent_rate.set_index('sent')
        sent_rate['rate'] = [0] * len(sent_rate.index)
        sent_rate = sent_rate.resample('5S').count()
        sent_rate['rate'] = sent_rate['rate'].divide(5)
        if sent_rate['rate'].max() > rateMax:
            rateMax = sent_rate['rate'].max()
        ax2.plot_date(sent_rate.index, sent_rate['rate'], linestyle='--', marker='', color='black', label='sent')
        ax2.set_ylim(0, rateMax+rateMax*0.1)
        ax2.set_ylabel('Rate (per second)')

        # calculate and plot latency quantiles
        calls = calls.set_index('finished')
        calls = calls.sort_index()
        quan = pandas.DataFrame(calls['took'])
        for q, c in [[.5, 'green'], [.9, 'orange'], [.99, 'red']]:
            quanN = quan.rolling(500, center=True).quantile(q)
            ax3.plot(quanN['took'].index, quanN['took'], color=c)

        ax3.set_ylabel('Latency quantiles (ms)')

        i += 1

    # format x axes
    for ax in fig.axes:
        matplotlib.pyplot.sca(ax)
        plt.xticks(rotation=30, ha='right')
        majorFormatter = matplotlib.dates.DateFormatter('%H:%M:%S')
        ax.xaxis.set_major_formatter(majorFormatter)

    # save image
    gs.update(wspace=0.275, hspace=0.5)
    fig.savefig(outputPath, bbox_inches='tight')

# and the main event
parser = argparse.ArgumentParser()
parser.add_argument('chartData', type=str, help='Path to file containing JSON chart output from load-generator')
parser.add_argument('--output', type=str, help='Path to save output to', default='latency-chart.png')
parser.add_argument('--title', type=str, help='Chart title')
args = parser.parse_args()

with open(args.chartData) as data_file:
    stuff = []
    for l in data_file.readlines():
        stuff.append(json.loads(l))

df = pandas.DataFrame(stuff)
df['finished'] = pandas.to_datetime(df['finished']).astype(datetime.datetime)
df['sent'] = pandas.to_datetime(df['sent']).astype(datetime.datetime)
df['took'] = df['took'].divide(1000000)

plot_section(df, args.title, args.output)
