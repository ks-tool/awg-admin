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

import {useMemo} from 'react';
import {projectMercator} from '@zenith-visuals/maps';
import land from './ne_110m_land.json';

type Ring = number[][];
type Polygon = Ring[];

interface LandData {
    features: {
        bbox: number[];
        geometry: {
            type: 'Polygon' | 'MultiPolygon';
            coordinates: Polygon | Polygon[];
        };
    }[];
}

interface WorldLandProps {
    width: number;
    height: number;
    transform: string;
}

export function WorldLand({width, height, transform}: WorldLandProps) {
    const path = useMemo(() => {
        if (width === 0) return '';
        return (land as LandData).features.filter(feature => feature.bbox[3] >= -60).map(({geometry}) => {
            const polygons = geometry.type === 'Polygon' ? [geometry.coordinates as Polygon] : geometry.coordinates as Polygon[];
            return polygons.map(polygon => polygon.map(ring => ring.map(([lon, lat], index) => {
                const point = projectMercator({lon, lat}, width, height);
                return `${index === 0 ? 'M' : 'L'}${point.x.toFixed(2)},${point.y.toFixed(2)}`;
            }).join(' ') + ' Z').join(' ')).join(' ');
        }).join(' ');
    }, [width, height]);

    return (
        <svg aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-visible" width={width} height={height} style={{transform, transformOrigin: '0 0'}}>
            <path d={path} fill="var(--muted-foreground)" fillOpacity={0.3} stroke="var(--border)" strokeWidth={0.75} />
        </svg>
    );
}
