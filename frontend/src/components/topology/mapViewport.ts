/*
  Copyright © 2026 Alexey Shulutkov <github@shulutkov.ru>

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  	http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

import {projectMercator} from '@zenith-visuals/maps';

const mapPadding = 16;
const northernmostLand = 83.64513;
const southernmostLand = -55.61183;

export interface MapFit {
    scale: number;
    x: number;
    y: number;
    top: number;
    bottom: number;
}

export interface MapPan {
    x: number;
    y: number;
}

export function fitMapViewport(width: number, height: number): MapFit {
    const top = projectMercator({lat: northernmostLand, lon: 0}, width, height).y;
    const bottom = projectMercator({lat: southernmostLand, lon: 0}, width, height).y;
    const contentHeight = bottom - top;
    const scale = width > 0 && height > 0 && contentHeight > 0
        ? Math.min(1, (width - mapPadding * 2) / width, (height - mapPadding * 2) / contentHeight)
        : 1;
    return {
        scale,
        x: (width - width * scale) / 2,
        y: (height - contentHeight * scale) / 2 - top * scale,
        top,
        bottom,
    };
}

export function clampMapPan(pan: MapPan, fit: MapFit, width: number, height: number, zoom: number): MapPan {
    const scale = fit.scale * zoom;
    const scaledWidth = width * scale;
    const scaledHeight = (fit.bottom - fit.top) * scale;
    const x = scaledWidth <= width - mapPadding * 2
        ? (width - scaledWidth) / 2
        : Math.min(mapPadding, Math.max(width - mapPadding - scaledWidth, pan.x));
    const y = scaledHeight <= height - mapPadding * 2
        ? (height - scaledHeight) / 2 - fit.top * scale
        : Math.min(mapPadding - fit.top * scale, Math.max(height - mapPadding - fit.bottom * scale, pan.y));
    return {x, y};
}

export function centeredMapPan(currentPan: MapPan, currentZoom: number, nextZoom: number, fit: MapFit, width: number, height: number): MapPan {
    if (nextZoom === 1) return {x: fit.x, y: fit.y};
    const ratio = nextZoom / currentZoom;
    return clampMapPan({
        x: width / 2 - (width / 2 - currentPan.x) * ratio,
        y: height / 2 - (height / 2 - currentPan.y) * ratio,
    }, fit, width, height, nextZoom);
}
